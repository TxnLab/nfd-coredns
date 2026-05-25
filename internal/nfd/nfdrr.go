/*
 * Copyright (c) 2024-2026. TxnLab Inc.
 * All Rights reserved.
 */

package nfd

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/miekg/dns"

	clog "github.com/coredns/coredns/plugin/pkg/log"
)

var (
	ErrNfdTooManySegments = errors.New("too many segments")
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
		//
		// Note: leading underscore-prefix labels (RFC 8552, e.g. _hayai._tcp)
		// don't shift this segment-name window — those labels are absorbed at
		// match time inside the segment's records (stored as e.g. "_hayai._tcp.@").
		// They're only used by the depth check below, which discounts them so
		// that _<svc>._<proto>.<segment>.<root>.algo can resolve normally.
		// Names that look like NFD names but aren't (e.g., "_tcp.foo.algo" when
		// no real segment "_tcp" exists) get filtered out by isValidNFDName
		// inside fetchNFDs and silently skipped.
	}
	// RFC 8552: leading '_'-prefixed labels are service-binding prefixes (SRV,
	// DKIM, etc.), not NFD segments — underscores aren't valid in NFD names.
	// Don't count them against the depth limit, so queries like
	// _test._tcp.bar.foo.algo can resolve against a segmented NFD.
	underscorePrefixCount := 0
	for underscorePrefixCount < len(qnameSplit) && strings.HasPrefix(qnameSplit[underscorePrefixCount], "_") {
		underscorePrefixCount++
	}
	if len(qnameSplit)-underscorePrefixCount > 4 {
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
	var segmentDelegated bool
	if segmentBasename != "" {
		var segmentFound bool
		nfdSegmentData, segmentFound = nfdData[segmentFQName]
		if segmentFound {
			// A segment is its own NFD with its own owner, and it always serves
			// its own DNS records — we never fail the lookup just because the
			// owners differ. How it combines with the root depends on ownership:
			//   - same owner: the root may extend the segment with sub-records
			//     (merged below; root wins on conflict).
			//   - different owner: the segment is a delegated child zone, solely
			//     authoritative for its own subtree. The root NFD owner has no
			//     DNS authority inside it (ie: mail.patrick.algo owned by someone
			//     other than patrick is mail's to control, not patrick's).
			segmentDelegated = nfdSegmentData.Internal["owner"] != nfdRootData.Internal["owner"]
			segmentJsonRrs, err = nfdToJsonRRs(ctx, nfdSegmentData)
			if err != nil {
				log.Errorf("error converting NFD:%s w/ dns prop:%s to jsonRRs: %v", segmentFQName, nfdSegmentData.UserDefined["dns"], err)
				return nil, err
			}
			// process the names (@ turns into FQDN) before the merge decision
			ConvertOriginRefs(ctx, segmentFQName, segmentJsonRrs)
		}
	}

	var mergedJsonRrs []JsonRr
	if segmentDelegated {
		// Delegated child zone: the queried name is always at or under the
		// segment, and the root has no authority inside the segment's subtree,
		// so the root's records are irrelevant here — serve only the segment's.
		// Skip building the root's records entirely (the root's DNS validity is
		// likewise irrelevant to a delegated segment).
		mergedJsonRrs = segmentJsonRrs
	} else {
		// Root, or a same-owner segment the root may extend: build the root's
		// records and merge (root wins on conflict).
		baseJsonRrs, err = nfdToJsonRRs(ctx, nfdRootData)
		if err != nil {
			log.Errorf("error converting NFD:%s w/ dns prop:%s to jsonRRs: %v", nfdRootName, nfdRootData.UserDefined["dns"], err)
			return nil, err
		}
		ConvertOriginRefs(ctx, nfdRootName, baseJsonRrs)
		mergedJsonRrs = MergeJsonRrrs(ctx, baseJsonRrs, segmentJsonRrs)
	}
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
