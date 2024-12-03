package nfd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/miekg/dns"
)

type NfdCache struct {
	*nfdFetcher
	nfdCache *expirable.LRU[string, NFDProperties]
	rrCache  *expirable.LRU[string, []JsonRr]
}

func NewNfdCache(client *algod.Client, registryID uint64) *NfdCache {
	return &NfdCache{
		nfdFetcher: newNfdFetcher(client, registryID),
		nfdCache:   expirable.NewLRU[string, NFDProperties](50000, nil, 1*time.Minute),
		rrCache:    expirable.NewLRU[string, []JsonRr](50000, nil, 1*time.Minute),
	}
}

func (n *NfdCache) FetchNFDs(ctx context.Context, names []string) (map[string]NFDProperties, error) {
	// Check cache - fetching only what's needed - combining results at end
	retVals := map[string]NFDProperties{}
	namesToFetch := make([]string, 0, len(names))
	for _, name := range names {
		props, found := n.nfdCache.Get(name)
		if !found {
			namesToFetch = append(namesToFetch, name)
			continue
		}
		retVals[name] = props
	}
	if len(namesToFetch) == 0 {
		return retVals, nil
	}
	fetchedNfds, err := n.nfdFetcher.FetchNfdDnsVal(ctx, namesToFetch)
	if err != nil {
		return nil, err
	}
	// merge the retVals with fetchedNfds map
	for name, props := range fetchedNfds {
		n.nfdCache.Add(name, props)
		retVals[name] = props
	}
	return retVals, nil
}

func (n *NfdCache) GetNfdRRs(ctx context.Context, log clog.P, qname string) ([]JsonRr, error) {
	var (
		qnameSplit      = dns.SplitDomainName(qname)
		nfdRootName     string
		segmentBasename string
		segmentFQName   string
		nfdsToFetch     []string
		nfdRoot         NFDProperties
		nfdSegment      NFDProperties
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
		// it could be a segment, or a record but either way the segment HAS to be looked up to determine
		// if it exists, and if so, does it have same owner.
	}
	if len(qnameSplit) > 4 {
		// ie: don't allow more than single RR name off of segment,
		// key.segment.patrick.algo
		return nil, fmt.Errorf("too many segments")

	}
	nfdData, err := n.FetchNFDs(ctx, nfdsToFetch)
	if err != nil {
		if errors.Is(err, ErrNfdNotFound) {
			return nil, err
		} else {
			log.Warningf("nfds %v error in fetch: %v", nfdsToFetch, err)
			return nil, err
		}
	}
	nfdRoot = nfdData[nfdRootName]
	if nfdRoot.Internal["name"] != nfdRootName {
		log.Errorf("nfdRoot.Internal.name: %s != %s", nfdRoot.Internal["name"], nfdRootName)
		return nil, fmt.Errorf("nfdRoot.Internal.name: %s != %s", nfdRoot.Internal["name"], nfdRootName)
	}
	var (
		baseJsonRrs    []JsonRr
		segmentJsonRrs []JsonRr
	)
	baseJsonRrs, err = NfdToJsonRRs(ctx, nfdRoot)
	if err != nil {
		log.Errorf("error converting NFD:%s w/ dns prop:%s to jsonRRs: %v", nfdRootName, nfdRoot.UserDefined["dns"], err)
		return nil, err
	}
	if segmentBasename != "" {
		var segmentFound bool
		nfdSegment, segmentFound = nfdData[segmentFQName]
		if segmentFound {
			// segment found - it MUST be same owner !!! so... can't set this record..
			// ie: mail.patrick.algo.xyz - but mail isn't owned by patrick
			// so we should act like it doesn't exist.
			if nfdSegment.Internal["owner"] != nfdRoot.Internal["owner"] {
				log.Warningf("nfdSegment.Internal.owner: %s != %s", nfdSegment.Internal["owner"], nfdRoot.Internal["owner"])
				return nil, ErrNfdSplitOwnership
			}
			segmentJsonRrs, err = NfdToJsonRRs(ctx, nfdSegment)
			if err != nil {
				log.Errorf("error converting NFD:%s w/ dns prop:%s to jsonRRs: %v", segmentFQName, nfdSegment.UserDefined["dns"], err)
				return nil, err
			}
		}
	}

	// we loaded the RRs from the NFDs - now process the names (@ turns into FQDN) and then merge
	ConvertOriginRefs(ctx, nfdRootName, baseJsonRrs)
	ConvertOriginRefs(ctx, segmentFQName, segmentJsonRrs)

	mergedJsonRrs := MergeJsonRrrs(ctx, baseJsonRrs, segmentJsonRrs)
	n.rrCache.Add(qname, mergedJsonRrs)

	return mergedJsonRrs, nil
}
