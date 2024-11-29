package nfd_coredns

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/miekg/dns"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
)

// NfdPlugin is a plugin that returns information held in the Ethereum Name Service.
type NfdPlugin struct {
	Next           plugin.Handler
	Client         *algod.Client
	RegistryID     uint64
	nfdNameServers []string
}

const (
	pluginName = "nfd"
)

var (
	log = clog.NewWithPlugin(pluginName)
	//defaultTtl = time.Duration(5 * time.Minute).Seconds()
	defaultTtl = 5 * 60
)

// Name implements the Handler interface.
func (n *NfdPlugin) Name() string { return pluginName }

// ServeDNS implements the plugin.Handler interface. This method gets called when example is used
// in a Server.
func (n *NfdPlugin) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	var (
		err        error
		qnameSplit []string
	)
	state := request.Request{W: w, Req: r}

	qname := state.Name()
	qtype := state.QType()

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	m.Compress = true
	m.Rcode = dns.RcodeSuccess

	if qtype != dns.TypeSOA && qtype != dns.TypeNS {
		// parse out the domain - needs to be [...].{root}.algo at minimum unless NS or SOA query
		qnameSplit = dns.SplitDomainName(qname)
		log.Infof("type:%s qnameSplit: %#+v", dns.TypeToString[qtype], qnameSplit)
		if len(qnameSplit) < 2 || qnameSplit[len(qnameSplit)-1] != "algo" {
			return plugin.NextOrFailure(n.Name(), n.Next, ctx, w, r)
		}
	}

	switch qtype {
	case dns.TypeSOA:
		retRrs, err := n.handleSOA(qname)
		if err != nil {
			log.Errorf("error handling SOA record: %v", err)
			return plugin.NextOrFailure(n.Name(), n.Next, ctx, w, r)
		}
		m.Answer = retRrs
		w.WriteMsg(m)
		return dns.RcodeSuccess, nil
	case dns.TypeNS:
		retRrs, err := n.handleNS(qname)
		if err != nil {
			log.Errorf("error handling NS record: %v", err)
			return plugin.NextOrFailure(n.Name(), n.Next, ctx, w, r)
		}
		m.Answer = retRrs
		w.WriteMsg(m)
		return dns.RcodeSuccess, nil
	case dns.TypeA:
	case dns.TypeAAAA:
	case dns.TypeCNAME:
	case dns.TypeTXT:
	case dns.TypeMX:
	case dns.TypeCAA:
	default:
		m.Rcode = dns.RcodeNotImplemented
		w.WriteMsg(m)
		return dns.RcodeNotImplemented, err
	}

	// Now fetch the root (and possibly segment) NFDs
	var (
		nfdRootName     string
		segmentBasename string
		segmentFQName   string
		nfdsToFetch     []string
		nfdRoot         NFDProperties
		nfdSegment      NFDProperties
	)
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
	nfdData, err := n.FetchNFDs(ctx, nfdsToFetch)
	if err != nil {
		if errors.Is(err, errNfdNotFound) {
			log.Warningf("nfds %v not found", nfdsToFetch)
			m.Rcode = dns.RcodeNameError
			w.WriteMsg(m)
			return dns.RcodeNameError, nil
		} else {
			log.Warningf("nfds %v error in fetch: %v", nfdsToFetch, err)
			m.Rcode = dns.RcodeFormatError
			w.WriteMsg(m)
			return dns.RcodeFormatError, nil
		}
	}
	nfdRoot = nfdData[nfdRootName]
	if nfdRoot.Internal["name"] != nfdRootName {
		log.Errorf("nfdRoot.Internal.name: %s != %s", nfdRoot.Internal["name"], nfdRootName)
		return plugin.NextOrFailure(n.Name(), n.Next, ctx, w, r)
	}
	var (
		baseJsonRrs    []JsonRr
		segmentJsonRrs []JsonRr
	)
	baseJsonRrs, err = NfdToJsonRRs(ctx, nfdRoot)
	if err != nil {
		log.Errorf("error converting NFD:%s w/ dns prop:%s to jsonRRs: %v", nfdRootName, nfdRoot.UserDefined["dns"], err)
		return plugin.NextOrFailure(n.Name(), n.Next, ctx, w, r)
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
				return plugin.NextOrFailure(n.Name(), n.Next, ctx, w, r)
			}
			segmentJsonRrs, err = NfdToJsonRRs(ctx, nfdSegment)
			if err != nil {
				log.Errorf("error converting NFD:%s w/ dns prop:%s to jsonRRs: %v", segmentFQName, nfdSegment.UserDefined["dns"], err)
				return plugin.NextOrFailure(n.Name(), n.Next, ctx, w, r)
			}
		}
	}

	// we loaded the RRs from the NFDs - now process the names (@ turns into FQDN) and then merge
	convertOriginRefs(ctx, nfdRootName, baseJsonRrs)
	convertOriginRefs(ctx, segmentFQName, segmentJsonRrs)

	mergedJsonRrs := mergeJsonRrrs(ctx, baseJsonRrs, segmentJsonRrs)

	retRRs, err := DnsRRsFromJsonRRs(mergedJsonRrs, qname, qtype)
	if err != nil {
		log.Errorf("error converting jsonRRs to dnsRRs: %v", err)
		return plugin.NextOrFailure(n.Name(), n.Next, ctx, w, r)
	}

	m.Answer = retRRs
	return dns.RcodeSuccess, nil
}

func (n *NfdPlugin) handleSOA(qname string) ([]dns.RR, error) {
	now := time.Now()
	ser := ((now.Hour()*3600 + now.Minute()) * 100) / 86400
	dateStr := fmt.Sprintf("%04d%02d%02d%02d", now.Year(), now.Month(), now.Day(), ser)

	var results []dns.RR
	if len(n.nfdNameServers) > 0 {
		// Create a synthetic SOA record (borrowed from coreens eg)
		result, err := dns.NewRR(fmt.Sprintf("%s 10800 IN SOA %s hostmaster.%s %s 3600 600 1209600 300", qname, n.nfdNameServers[0], n.nfdNameServers[0], dateStr))
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (n *NfdPlugin) handleNS(qname string) ([]dns.RR, error) {
	results := make([]dns.RR, 0)
	for _, nameserver := range n.nfdNameServers {
		result, err := dns.NewRR(fmt.Sprintf("%s 3600 IN NS %s", qname, nameserver))
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}

	return results, nil
}
