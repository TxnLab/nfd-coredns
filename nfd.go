/*
 * Copyright (c) 2024. TxnLab Inc.
 * All Rights reserved.
 */

package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/miekg/dns"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/nonwriter"
	"github.com/coredns/coredns/request"

	"github.com/TxnLab/nfd-coredns/internal/nfd"
)

// NfdPlugin is a plugin that returns information held in the Ethereum Name Service.
type NfdPlugin struct {
	Next           plugin.Handler
	Forwarder      plugin.Handler
	NfdCache       *nfd.Cache
	nfdNameServers []string
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
	//defaultTtl = time.Duration(5 * time.Minute).Seconds()
	defaultTtl = 5 * 60

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
	case NoData:
		if n.Next == nil {
			log.Debugf("No data for %s, no next plugin", state.Name())
			state.SizeAndDo(a)
			w.WriteMsg(a)
			return dns.RcodeSuccess, nil
		}
		log.Debugf("No data for %s, delegating to next plugin", state.Name())
		return plugin.NextOrFailure(n.Name(), n.Next, ctx, w, r)
	case NameError:
		log.Warningf("name error for %s", state.Name())
		a.Rcode = dns.RcodeNameError
	case ServerFailure:
		log.Warningf("server failure for %s", state.Name())
		a.Rcode = dns.RcodeServerFailure
		return dns.RcodeServerFailure, nil
	case NotImplemented:
		return dns.RcodeNotImplemented, nil
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
	if qtype == dns.TypeSOA || qtype == dns.TypeNS {
		rrs, err := n.Query([]nfd.JsonRr{}, qname, qtype)
		if err != nil {
			return nil, nil, nil, ServerFailure
		}
		if len(rrs) == 0 {
			return nil, nil, nil, NoData
		}
		return rrs, nil, nil, Success
	}
	// needs to be [...].{root}.algo at minimum unless NS or SOA query
	if len(qnameSplit) < 2 || qnameSplit[len(qnameSplit)-1] != "algo" {
		// We should only have gotten here if via internal CNAME -> lookup process - so do resolve via
		// external forwarder (google)
		return n.LookupViaForwarder(ctx, state)
	}
	// Now fetch the root (and possibly segment) NFDs to determine which NFD the data
	// is being fetched from root (direclty) - root w/ nested data, or segment w or w/o further sub-data
	var (
		answerRrs     []dns.RR
		authorityRrs  []dns.RR
		additionalRrs []dns.RR
	)
	mergedJsonRrs, err := n.NfdCache.GetNfdRRs(ctx, log, qname)
	if errors.Is(err, nfd.ErrNfdNotFound) || (err == nil && mergedJsonRrs == nil) {
		return nil, nil, nil, NoData
	}
	if err != nil {
		log.Errorf("error getting NFDs: %v", err)
		return nil, nil, nil, ServerFailure
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
			}
			log.Warning("cname derived lookup failed - returning original answerRrs")
			return answerRrs, authorityRrs, additionalRrs, targetResult
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

// Query  the query type against our combined records - name and qtype have to match
func (n *NfdPlugin) Query(jsonRecords []nfd.JsonRr, queryName string, qType uint16) ([]dns.RR, error) {
	switch qType {
	case dns.TypeSOA:
		return n.handleSOA(queryName)
	case dns.TypeNS:
		return n.handleNS(queryName)
	case dns.TypeCAA:
		fallthrough
	case dns.TypeCNAME:
		fallthrough
	case dns.TypeTXT:
		fallthrough
	case dns.TypeMX:
		fallthrough
	case dns.TypeA:
		fallthrough
	case dns.TypeAAAA:
		return nfd.DnsRRsFromJsonRRs(jsonRecords, queryName, qType)
	default:
		return nil, errNotImplemented
	}
}

func (n *NfdPlugin) handleSOA(qName string) ([]dns.RR, error) {
	now := time.Now()
	ser := ((now.Hour()*3600 + now.Minute()) * 100) / 86400
	dateStr := fmt.Sprintf("%04d%02d%02d%02d", now.Year(), now.Month(), now.Day(), ser)

	var results []dns.RR
	if len(n.nfdNameServers) > 0 {
		// Create a synthetic SOA record (borrowed from coreens eg)
		result, err := dns.NewRR(fmt.Sprintf("%s 10800 IN SOA %s hostmaster.%s %s 3600 600 1209600 300", qName, n.nfdNameServers[0], n.nfdNameServers[0], dateStr))
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

func (n *NfdPlugin) LookupViaForwarder(ctx context.Context, state request.Request) ([]dns.RR, []dns.RR, []dns.RR, Result) {
	writer := nonwriter.New(state.W)

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
