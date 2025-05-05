/*
 * Copyright (c) 2024. TxnLab Inc.
 * All Rights reserved.
 */

package nfd

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/miekg/dns"

	clog "github.com/coredns/coredns/plugin/pkg/log"
)

var (
	ErrNfdTooManySegments = errors.New("too many segments")
	ErrNfdSplitOwnership  = errors.New("nfd segment has different owner than root")
)

// NfdRRHandler is interface used for fetching DNS resource resources from an NFD, returned as a slice of JsonRR's
type NfdRRHandler interface {
	GetNfdRRs(ctx context.Context, log clog.P, qname string) ([]JsonRr, error)
}
type nfdRRHandler struct {
	nfdFetcher NfdFetcher
	nfdCache   *expirable.LRU[string, Properties]
	rrCache    *expirable.LRU[string, []JsonRr]
}

func NewNfdRRHandler(client *algod.Client, registryID uint64, algoXyzIp string, cacheTtl time.Duration) NfdRRHandler {
	return &nfdRRHandler{
		nfdFetcher: newNfdFetcher(client, registryID, algoXyzIp),
		nfdCache:   expirable.NewLRU[string, Properties](50000, nil, cacheTtl),
		rrCache:    expirable.NewLRU[string, []JsonRr](50000, nil, cacheTtl),
	}
}

func (n *nfdRRHandler) GetNfdRRs(ctx context.Context, log clog.P, qname string) ([]JsonRr, error) {
	var (
		qnameSplit      = dns.SplitDomainName(qname)
		nfdRootName     string
		segmentBasename string
		segmentFQName   string
		nfdsToFetch     = make([]string, 0, len(qnameSplit)-1)
		nfdRootData     Properties
		nfdSegmentData  Properties
	)
	rrs, found := n.rrCache.Get(qname)
	if found {
		return rrs, nil
	}

	if qnameSplit[len(qnameSplit)-1] != "algo" {
		return nil, fmt.Errorf("qnameSplit[len(qnameSplit)-1] != algo")
	}
	nfdRootName = qnameSplit[len(qnameSplit)-2] + ".algo"
	nfdsToFetch = append(nfdsToFetch, nfdRootName)
	if len(qnameSplit) > 2 {
		segmentBasename = qnameSplit[len(qnameSplit)-3]
		segmentFQName = segmentBasename + "." + nfdRootName
		nfdsToFetch = append(nfdsToFetch, segmentFQName)
		// ie: mail.patrick.algo -  segmentBasename would be 'mail'
		// it could be a segment, or a record, but either way the segment HAS to be looked up to determine
		// if it exists, and if so, does it have the same owner.
	}
	if len(qnameSplit) > 4 {
		// ie: don't allow more than a single RR name off of segment ?
		// key.segment.patrick.algo
		return nil, ErrNfdTooManySegments

	}
	// fetch (valid) NFDs (ie: _atproto.patrick.algo won't try to fetch _atproto.patrick.algo as a segment)
	nfdData, err := n.fetchNFDs(ctx, log, nfdsToFetch)
	if err != nil {
		if errors.Is(err, ErrNfdNotFound) {
			log.Infof("nfd %v not found: %v", nfdsToFetch, err)
			return nil, err
		} else {
			log.Warningf("nfds %v error in fetch: %v", nfdsToFetch, err)
			return nil, err
		}
	}
	nfdRootData = nfdData[nfdRootName]
	if nfdRootData.Internal["name"] != nfdRootName {
		log.Errorf("nfdRootData.Internal.name: %s != %s", nfdRootData.Internal["name"], nfdRootName)
		return nil, fmt.Errorf("nfdRootData.Internal.name: %s != %s", nfdRootData.Internal["name"], nfdRootName)
	}
	var (
		baseJsonRrs    []JsonRr
		segmentJsonRrs []JsonRr
	)
	baseJsonRrs, err = nfdToJsonRRs(ctx, nfdRootData)
	if err != nil {
		log.Errorf("error converting NFD:%s w/ dns prop:%s to jsonRRs: %v", nfdRootName, nfdRootData.UserDefined["dns"], err)
		return nil, err
	}
	if segmentBasename != "" {
		var segmentFound bool
		nfdSegmentData, segmentFound = nfdData[segmentFQName]
		if segmentFound {
			// segment found - it MUST be the same owner !!! so... can't set this record..
			// ie: mail.patrick.algo.xyz - but mail isn't owned by patrick
			// so we should act like it doesn't exist.
			if nfdSegmentData.Internal["owner"] != nfdRootData.Internal["owner"] {
				log.Warningf("nfdSegmentData.Internal.owner: (%s) %s != (%s) %s", nfdSegmentData.Internal["name"], nfdSegmentData.Internal["owner"],
					nfdRootData.Internal["name"], nfdRootData.Internal["owner"])
				return nil, ErrNfdSplitOwnership
			}
			segmentJsonRrs, err = nfdToJsonRRs(ctx, nfdSegmentData)
			if err != nil {
				log.Errorf("error converting NFD:%s w/ dns prop:%s to jsonRRs: %v", segmentFQName, nfdSegmentData.UserDefined["dns"], err)
				return nil, err
			}
		}
	}

	// we loaded the RRs from the NFDs - now process the names (@ turns into FQDN) and then merge
	ConvertOriginRefs(ctx, nfdRootName, baseJsonRrs)
	ConvertOriginRefs(ctx, segmentFQName, segmentJsonRrs)

	mergedJsonRrs := MergeJsonRrrs(ctx, baseJsonRrs, segmentJsonRrs)
	log.Debugf("mergedJsonRrs: %+v", mergedJsonRrs)
	n.rrCache.Add(qname, mergedJsonRrs)

	return mergedJsonRrs, nil
}

func (n *nfdRRHandler) fetchNFDs(ctx context.Context, log clog.P, names []string) (map[string]Properties, error) {
	// Check cache - fetching only what's needed - combining results at the end
	retVals := map[string]Properties{}
	namesToFetch := make([]string, 0, len(names))
	log.Debugf("fetchNFDs: names: %v", names)
	for _, name := range names {
		if !isValidNFDName(name) {
			continue
		}
		props, found := n.nfdCache.Get(name)
		if !found {
			namesToFetch = append(namesToFetch, name)
			continue
		}
		log.Debugf("found in nfd cache: %s, %d props", name, len(props.Internal)+len(props.UserDefined)+len(props.Verified))
		if len(props.Internal) == 0 {
			// fake 'not found' placeholder - don't try to fetch it again but don't add it to retVals either
			continue
		}
		retVals[name] = props
	}
	if len(namesToFetch) == 0 {
		// everything we need is in the cache - return it
		return retVals, nil
	}
	// fetch the list of nfds and merge with cache
	fetchedNfds, err := n.nfdFetcher.FetchNfdDnsVals(ctx, namesToFetch)
	log.Debugf("fetchedNfds: names to fetch:%v, fetched:%d, %v, err:%v", namesToFetch, len(fetchedNfds), slices.Collect(maps.Keys(fetchedNfds)), err)
	// Add the names that were NOT found to our cache - but as not-found so we don't keep trying to fetch them for a bit
	for _, name := range namesToFetch {
		var found bool
		if fetchedNfds == nil {
			found = false
		} else {
			_, found = fetchedNfds[name]
		}
		if !found {
			log.Debugf("[not found] added to nfd cache: %s, 0 props", name)
			n.nfdCache.Add(name, Properties{})
		}
	}

	if errors.Is(err, ErrNfdNotFound) {
		if len(retVals) > 0 {
			// return the cached values we already set into retVals
			return retVals, nil
		}
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	// merge the prior cached retVals with fetchedNfds map
	for name, props := range fetchedNfds {
		n.nfdCache.Add(name, props)
		log.Debugf("added to nfd cache: %s, %d props", name, len(props.Internal)+len(props.UserDefined)+len(props.Verified))
		retVals[name] = props
	}
	return retVals, nil
}
