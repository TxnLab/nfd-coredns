/*
 * Copyright (c) 2024-2026. TxnLab Inc.
 * All Rights reserved.
 */

package main

import (
	"context"
	"errors"
	"strings"

	"github.com/miekg/dns"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/nonwriter"
	"github.com/coredns/coredns/request"

	"github.com/TxnLab/nfd-coredns/internal/nfd"
)

// NfdPlugin is a plugin that returns information held in the Ethereum Name Service.
type NfdPlugin struct {
	Next       plugin.Handler
	Forwarder  plugin.Handler
	NfdHandler nfd.NfdRRHandler
	zoneSOA    dns.RR // SOA record for negative responses (RFC 2308)
	zoneOrigin string // Server block zone origin (e.g., "algo.xyz." or "dotalgo.io.")
}

// Result of a lookup
type Result int

const (
	pluginName = "nfd"

	// Success is a successful lookup.
	Success Result = iota
	// NameError indicates a nameerror
	NameError
	// Delegation indicates the lookup resulted in a delegation.
	Delegation
	// NoData indicates the lookup resulted in a NODATA.
	NoData
	// ServerFailure indicates a server failure during the lookup.
	ServerFailure
	// NotImplemented is for unsupported RR types
	NotImplemented
)

var (
	log = clog.NewWithPlugin(pluginName)

	errNotImplemented = errors.New("not implemented")
)

// Name implements the CoreDNS plugin.Handler interface.
func (n *NfdPlugin) Name() string { return pluginName }

// ServeDNS implements the CoreDNS plugin.Handler interface
func (n *NfdPlugin) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	a := new(dns.Msg)
	a.SetReply(r)
	a.Compress = true
	a.Authoritative = true
	var result Result
	a.Answer, a.Ns, a.Extra, result = n.Lookup(ctx, state)
	switch result {
	case Success:
		state.SizeAndDo(a)
		w.WriteMsg(a)
		return dns.RcodeSuccess, nil
	case Delegation:
		// Root zone subdomain (e.g., _psl.algo after rewrite from _psl.algo.xyz).
		// Rewrite the query name back to the original zone form so the file plugin
		// can match its zone and serve the record.
		if n.Next != nil && n.zoneOrigin != "" {
			delegateR := r.Copy()
			qname := state.Name()
			labels := dns.SplitDomainName(qname)
			if len(labels) >= 2 {
				prefix := strings.Join(labels[:len(labels)-1], ".")
				delegateR.Question[0].Name = prefix + "." + n.zoneOrigin
			}
			log.Debugf("Delegation for %s, rewriting to %s and delegating to %s", qname, delegateR.Question[0].Name, n.Next.Name())
			return plugin.NextOrFailure(n.Name(), n.Next, ctx, w, delegateR)
		}
		// Fallthrough to NoData if no next plugin or zone origin
		state.SizeAndDo(a)
		w.WriteMsg(a)
		return dns.RcodeSuccess, nil
	case NoData:
		if n.Next == nil {
			log.Debugf("No data for %s (%s), no next plugin", state.Name(), state.Type())
			state.SizeAndDo(a)
			w.WriteMsg(a)
			return dns.RcodeSuccess, nil
		}
		log.Debugf("No data for %s (%s), delegating to next plugin:%s", state.Name(), state.Type(), n.Next.Name())
		return plugin.NextOrFailure(n.Name(), n.Next, ctx, w, r)
	case NameError:
		log.Warningf("name error for %s, returning RcodeNameError (NXDomain)", state.Name())
		a.Rcode = dns.RcodeNameError
	case ServerFailure:
		log.Warningf("server failure for %s (ServFail)", state.Name())
		a.Rcode = dns.RcodeServerFailure
		return dns.RcodeServerFailure, nil
	case NotImplemented:
		return dns.RcodeNotImplemented, nil
	default:
		log.Warningf("unknown result for %s, returning RcodeServerFailure (ServFail)", state.Name())
		a.Rcode = dns.RcodeServerFailure
	}
	// Unknown result...
	return dns.RcodeServerFailure, nil
}

func (n *NfdPlugin) Lookup(ctx context.Context, state request.Request) ([]dns.RR, []dns.RR, []dns.RR, Result) {
	var (
		err        error
		qnameSplit []string
	)
	qname := state.Name()
	qtype := state.QType()

	//  Kick out unsupported qtype's immediately
	switch qtype {
	case dns.TypeSOA:
		fallthrough
	case dns.TypeNS:
		fallthrough
	case dns.TypeCAA:
		fallthrough
	case dns.TypeCNAME:
		fallthrough
	case dns.TypeTXT:
		fallthrough
	case dns.TypeMX:
		fallthrough
	case dns.TypeSRV:
		fallthrough
	case dns.TypeCERT:
		fallthrough
	case dns.TypeA:
		fallthrough
	case dns.TypeAAAA:
	// we're ok... let passthrough
	default:
		return nil, nil, nil, NotImplemented
	}

	// parse out the domain into parts (won't have . terminator)
	qnameSplit = dns.SplitDomainName(qname)
	log.Infof("Lookup: qname: %s qtype: %s", qname, dns.TypeToString[qtype])
	if len(qnameSplit) < 2 {
		// We should only have gotten here if via internal CNAME -> lookup process - so do resolve via
		// external forwarder (google)
		return n.LookupViaForwarder(ctx, state)
	} else if qnameSplit[len(qnameSplit)-1] != "algo" {
		// we'll get here for something like patrick.algo (that was originally patrick.algo.xyz but our rewrite rule turns it into patrick.algo)
		// we'll also get here for the ROOT zone fetch of the real name - like algo.xyz and dotalgo.io
		// so fallback to file plugin for our hardcoded root zone info for our root zones, otherwise forward to google
		if (qnameSplit[len(qnameSplit)-1] == "algo" || qnameSplit[len(qnameSplit)-1] == "xyz") || (qnameSplit[len(qnameSplit)-2] == "dotalgo" && qnameSplit[len(qnameSplit)-1] == "io") {
			return nil, nil, nil, NoData
		} else {
			// some other root - just do lookup via forwarder
			return n.LookupViaForwarder(ctx, state)
		}
	}
	// Root zone subdomain records (like _psl, _dmarc, _acme-challenge) should be
	// served from the embedded zone file, not looked up as NFD blockchain data.
	// Labels starting with '_' follow DNS service record convention (RFC 8552)
	// and are never valid NFD names (which only allow a-z0-9).
	if strings.HasPrefix(qnameSplit[len(qnameSplit)-2], "_") {
		return nil, nil, nil, Delegation
	}

	// Now fetch the root (and possibly segment) NFDs to determine which NFD the data
	// is being fetched from root (directly) - root w/ nested data, or segment w or w/o further sub-data
	// ie, patrick.algo [root], or foo.patrick.algo [contained within patrick.algo as RR value], or foo.patrick.algo [segment off patrick.algo]
	var (
		answerRrs     []dns.RR
		authorityRrs  []dns.RR
		additionalRrs []dns.RR
	)
	mergedJsonRrs, err := n.NfdHandler.GetNfdRRs(ctx, log, qname)
	if errors.Is(err, nfd.ErrNfdNotFound) || (err == nil && mergedJsonRrs == nil) {
		return nil, nil, nil, NoData
	}
	if err != nil {
		log.Errorf("error getting NFD: %s: %v", qname, err)
		return nil, nil, nil, ServerFailure
	}

	// NS queries on NFD subdomains return Success with empty answer + SOA in authority
	// Per RFC 2308: existing name with no records of requested type
	// NFDs are not delegated subzones and cannot define their own NS records
	if qtype == dns.TypeNS {
		if n.zoneSOA != nil {
			return nil, []dns.RR{n.zoneSOA}, nil, Success
		}
		return nil, nil, nil, NoData
	}

	// If we aren't asking for a CNAME then check for one to see if we need
	// to recurse
	if qtype != dns.TypeCNAME {
		cnameRrs, err := n.Query(mergedJsonRrs, qname, dns.TypeCNAME)
		if err != nil {
			return nil, nil, nil, ServerFailure
		}
		if len(cnameRrs) > 0 {
			// got cname - ie: user queried foo.bar (an A qtype) - has cname - and foo.bar points to foo.baz
			// so we'll do original A query on foo.baz
			answerRrs = append(answerRrs, cnameRrs...)

			cnameRec, ok := cnameRrs[0].(*dns.CNAME)
			if !ok {
				log.Errorf("error converting CNAME to dns.RR: val: %v", cnameRrs[0])
				return nil, nil, nil, ServerFailure
			}
			log.Infof("cnameRec: %#+v", cnameRec)
			targetReq := state.Req.Copy()
			targetReq.Question[0].Name = cnameRec.Target
			targetReq.Question[0].Qtype = qtype
			targetReqState := request.Request{W: state.W, Req: targetReq}
			// recurse...
			targetAnswerRrs, targetAuthorityRrs, targetAdditionalRrs, targetResult := n.Lookup(ctx, targetReqState)
			if targetResult == Success {
				log.Infof("cname derived lookup succeeded")
				answerRrs = append(answerRrs, targetAnswerRrs...)
				authorityRrs = append(authorityRrs, targetAuthorityRrs...)
				additionalRrs = append(additionalRrs, targetAdditionalRrs...)
				return answerRrs, authorityRrs, additionalRrs, Success
			}
			log.Warning("cname derived lookup failed - returning CNAME only")
			return answerRrs, authorityRrs, additionalRrs, Success
		}
	}

	// Match the query type against our combined records - name and qtype have to match
	rrs, err := n.Query(mergedJsonRrs, qname, qtype)
	if err != nil {
		if errors.Is(err, errNotImplemented) {
			return nil, nil, nil, NotImplemented
		}
		return nil, nil, nil, ServerFailure
	}
	if len(rrs) == 0 {
		return nil, nil, nil, NoData
	}
	answerRrs = append(answerRrs, rrs...)
	if len(answerRrs) == 0 {
		return answerRrs, authorityRrs, additionalRrs, NoData
	}

	return answerRrs, authorityRrs, additionalRrs, Success
}

// Query the query type against our combined records - name and qtype have to match
func (n *NfdPlugin) Query(jsonRecords []nfd.JsonRr, queryName string, qType uint16) ([]dns.RR, error) {
	// For root zone query, just fallthrough (will go to 'internal code' file zone)
	switch qType {
	case dns.TypeSOA:
		fallthrough
	case dns.TypeNS:
		fallthrough
	case dns.TypeCAA:
		fallthrough
	case dns.TypeCNAME:
		fallthrough
	case dns.TypeTXT:
		fallthrough
	case dns.TypeMX:
		fallthrough
	case dns.TypeSRV:
		fallthrough
	case dns.TypeCERT:
		fallthrough
	case dns.TypeA:
		fallthrough
	case dns.TypeAAAA:
		return nfd.DnsRRsFromJsonRRs(jsonRecords, queryName, qType)
	default:
		return nil, errNotImplemented
	}
}

func (n *NfdPlugin) LookupViaForwarder(ctx context.Context, state request.Request) ([]dns.RR, []dns.RR, []dns.RR, Result) {
	writer := nonwriter.New(state.W)

	log.Infof("LookupViaForwarder: qname: %s qtype: %s", state.Name(), dns.TypeToString[state.QType()])
	code, err := n.Forwarder.ServeDNS(ctx, writer, state.Req)
	if err != nil {
		log.Errorf("error resolving via forwarder: %v", err)
		return nil, nil, nil, ServerFailure
	}
	if code != dns.RcodeSuccess {
		log.Warningf("forwarder returned: %s", dns.RcodeToString[code])
		return nil, nil, nil, ServerFailure
	}
	return writer.Msg.Answer, nil, writer.Msg.Extra, Success
}
