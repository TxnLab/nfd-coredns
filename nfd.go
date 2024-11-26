package nfd_coredns

import (
	"context"
	"errors"

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
		err error
	)
	state := request.Request{W: w, Req: r}

	zone := plugin.Zones([]string{"algo.xyz."}).Matches(state.Name())
	if zone == "" {
		return plugin.NextOrFailure(n.Name(), n.Next, ctx, w, r)
	}

	qname := state.Name()
	qtype := state.QType()

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	m.Compress = true
	m.Rcode = dns.RcodeSuccess

	// parse out the domain - needs to be [...].{root}.algo.xyz at minimum
	qnameSplit := dns.SplitDomainName(qname)
	log.Infof("qnameSplit: %#+v", qnameSplit)
	if len(qnameSplit) < 3 || qnameSplit[len(qnameSplit)-2] != "algo" || qnameSplit[len(qnameSplit)-1] != "xyz" {
		return plugin.NextOrFailure(n.Name(), n.Next, ctx, w, r)
	}

	// handle what types we do don't support right off the bat
	switch dns.TypeToString[qtype] {
	case "TXT":
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
	nfdRootName = qnameSplit[len(qnameSplit)-3] + ".algo"
	nfdsToFetch = append(nfdsToFetch, nfdRootName)
	if len(qnameSplit) > 3 {
		segmentBasename = qnameSplit[len(qnameSplit)-4]
		segmentFQName = segmentBasename + "." + nfdRootName
		nfdsToFetch = append(nfdsToFetch, segmentFQName)
		// ie: mail.patrick.algo.xyz -  segmentBasename would be 'mail'
		// it could be a segment, or a record but either way the segment HAS to be looked up to determine
		// if it exists, and if so, does it have same owner.
	}
	nfdData, err := n.FetchNFDs(ctx, nfdsToFetch)
	//log.Infof("nfdData: %#+v", nfdData)
	if errors.Is(err, errNfdNotFound) {
		log.Warningf("nfds [%v] not found", nfdsToFetch)
		m.Rcode = dns.RcodeNameError
		w.WriteMsg(m)
		return dns.RcodeNameError, nil
	}
	nfdRoot = nfdData[nfdRootName]
	if nfdRoot.Internal["name"] != nfdRootName {
		log.Errorf("nfdRoot.Internal.name: %s != %s", nfdRoot.Internal["name"], nfdRootName)
		return plugin.NextOrFailure(n.Name(), n.Next, ctx, w, r)
	}

	// TODO - fetch json RR's from root NFD
	var (
		baseJsonRrs    []JsonRr
		segmentJsonRrs []JsonRr
	)

	if segmentBasename != "" {
		var segmentFound bool
		nfdSegment, segmentFound = nfdData[segmentFQName]
		if segmentFound {
			// segment found - it MUST be same owner !!! so... can't set this record..
			// ie: mail.patrick.algo.xyz - but mail isn't owned by patrick
			// so we should act like it doesn't exist.
			if nfdSegment.Internal["owner.a"] != nfdRoot.Internal["owner.a"] {
				log.Warningf("nfdSegment.Internal.owner.a: %s != %s", nfdSegment.Internal["owner.a"], nfdRoot.Internal["owner.a"])
				return plugin.NextOrFailure(n.Name(), n.Next, ctx, w, r)
			}
			// fetch json RRs from segment
			segmentJsonRrs = []JsonRr{}
		}
	}
	baseJsonRrs = []JsonRr{
		{
			Name: "patrick.algo.xyz.",
			Rrdatas: []string{
				"google-site-verification=Rfkw7MGNQiBhlpxWqRRzuWulK1ZTnQUlTrBTwDA7xv0",
				`"v=spf1 mx include:netblocks.dreamhost.com include:relay.mailchannels.net -all"`,
				"MS=ms44313055",
			},
			Ttl:  60,
			Type: "TXT",
		},
	}
	segmentJsonRrs = []JsonRr{
		{
			Name:    "foo.patrick.algo.xyz.",
			Rrdatas: []string{"nfd-verify=xxxx"},
			Ttl:     60,
			Type:    "TXT",
		},
	}

	mergedJsonRrs := mergeJsonRrrs(ctx, baseJsonRrs, segmentJsonRrs)

	retRRs, err := DnsRRsFromJsonRRs(mergedJsonRrs, qname, qtype)
	if err != nil {
		log.Errorf("error converting jsonRRs to dnsRRs: %v", err)
		return plugin.NextOrFailure(n.Name(), n.Next, ctx, w, r)
	}
	//log.Infof("retRRs: %#+v", retRRs)

	m.Answer = retRRs
	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

//func (n *NfdPlugin) handleTXT(name string, domain string, contentHash []byte) ([]dns.RR, error) {
//	var results []dns.RR
//
//	log.Infof("name: %s, domain: %s", name, domain)
//	if domain != "." {
//		return results, fmt.Errorf("domain:%s not expected", domain)
//	}
//	trimmedName := strings.TrimSuffix(name, ".")
//	nfdId, err := n.FindNFDAppIDByName(context.Background(), trimmedName)
//	if err != nil {
//		return results, err
//	}
//	nfd, err := n.FetchNFD(context.Background(), nfdId, false)
//	if err != nil {
//		return results, err
//	}
//	if txtRec := nfd.UserDefined["txt"]; txtRec != "" {
//		result, err := dns.NewRR(fmt.Sprintf("%s 300 IN TXT \"%s\"", name, txtRec))
//		if err != nil {
//			return results, err
//		}
//		results = append(results, result)
//	}
//	//return &dns.TXT{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: s.TTL}, Txt: split255(s.Text)}
//
//	results = append(results, &dns.TXT{
//		Hdr: dns.RR_Header{
//			Name:   name,
//			Rrtype: dns.TypeTXT,
//			Class:  dns.ClassINET,
//			Ttl:    uint32(defaultTtl),
//		},
//		Txt: []string{"dummy val"},
//	})
//
//	// if isRealOnChainDomain(name, domain) {
//	// 	ethDomain := strings.TrimSuffix(domain, ".")
//	// 	resolver, err := n.getResolver(ethDomain)
//	// 	if err != nil {
//	// 		log.Warnf("error obtaining resolver for %s: %v", ethDomain, err)
//	// 		return results, nil
//	// 	}
//	//
//	// 	address, err := resolver.Address()
//	// 	if err != nil {
//	// 		if err.Error() != "abi: unmarshalling empty output" {
//	// 			return results, err
//	// 		}
//	// 		return results, nil
//	// 	}
//	//
//	// 	if address != ens.UnknownAddress {
//	// 		result, err := dns.NewRR(fmt.Sprintf("%s 3600 IN TXT \"a=%s\"", name, address.Hex()))
//	// 		if err != nil {
//	// 			return results, err
//	// 		}
//	// 		results = append(results, result)
//	// 	}
//	//
//	// 	result, err := dns.NewRR(fmt.Sprintf("%s 3600 IN TXT \"contenthash=0x%x\"", name, contentHash))
//	// 	if err != nil {
//	// 		return results, err
//	// 	}
//	// 	results = append(results, result)
//	//
//	// 	// Also provide dnslink for compatibility with older IPFS gateways
//	// 	contentHashStr, err := ens.ContenthashToString(contentHash)
//	// 	if err != nil {
//	// 		return results, err
//	// 	}
//	// 	result, err = dns.NewRR(fmt.Sprintf("%s 3600 IN TXT \"dnslink=%s\"", name, contentHashStr))
//	// 	if err != nil {
//	// 		return results, nil
//	// 	}
//	// 	results = append(results, result)
//	// } else if isRealOnChainDomain(strings.TrimPrefix(name, "_dnslink."), domain) {
//	// 	// This is a request to _dnslink.<domain>, return the DNS link record.
//	// 	contentHashStr, err := ens.ContenthashToString(contentHash)
//	// 	if err != nil {
//	// 		return results, err
//	// 	}
//	// 	result, err := dns.NewRR(fmt.Sprintf("%s 3600 IN TXT \"dnslink=%s\"", name, contentHashStr))
//	// 	if err != nil {
//	// 		return results, err
//	// 	}
//	// 	results = append(results, result)
//	// }
//
//	return results, nil
//}
