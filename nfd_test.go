/*
 * Copyright (c) 2025-2026. TxnLab Inc.
 * All Rights reserved.
 */

package main

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/TxnLab/nfd-coredns/internal/nfd"
)

// mockNfdRRHandler implements nfd.NfdRRHandler for testing
type mockNfdRRHandler struct {
	rrs []nfd.JsonRr
	err error
}

func (m *mockNfdRRHandler) GetNfdRRs(ctx context.Context, log clog.P, qname string) ([]nfd.JsonRr, error) {
	return m.rrs, m.err
}

// mockForwarder implements plugin.Handler for testing
type mockForwarder struct {
	answer []dns.RR
	rcode  int
	err    error
}

func (m *mockForwarder) Name() string { return "mock-forwarder" }

func (m *mockForwarder) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if m.err != nil {
		return dns.RcodeServerFailure, m.err
	}
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Answer = m.answer
	w.WriteMsg(msg)
	return m.rcode, nil
}

// mockNextPlugin captures the rewritten query name for delegation tests
type mockNextPlugin struct {
	receivedName string
	answer       []dns.RR
}

func (m *mockNextPlugin) Name() string { return "mock-next" }

func (m *mockNextPlugin) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	m.receivedName = r.Question[0].Name
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Answer = m.answer
	w.WriteMsg(msg)
	return dns.RcodeSuccess, nil
}

// testResponseWriter implements dns.ResponseWriter for testing
type testResponseWriter struct {
	test.ResponseWriter
	msg *dns.Msg
}

func (t *testResponseWriter) WriteMsg(m *dns.Msg) error {
	t.msg = m
	return nil
}

func (t *testResponseWriter) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53}
}

func (t *testResponseWriter) RemoteAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
}

func TestServeDNS(t *testing.T) {
	tests := []struct {
		name           string
		qname          string
		qtype          uint16
		mockHandler    *mockNfdRRHandler
		expectedRcode  int
		expectedAnswer int
	}{
		{
			name:  "successful A record lookup",
			qname: "test.algo.",
			qtype: dns.TypeA,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "test.algo.", Type: "A", RrData: []string{"192.168.1.1"}, Ttl: 300},
				},
			},
			expectedRcode:  dns.RcodeSuccess,
			expectedAnswer: 1,
		},
		{
			name:  "NFD not found returns NoData",
			qname: "notfound.algo.",
			qtype: dns.TypeA,
			mockHandler: &mockNfdRRHandler{
				err: nfd.ErrNfdNotFound,
			},
			expectedRcode:  dns.RcodeSuccess, // NoData still returns success with no records
			expectedAnswer: 0,
		},
		{
			name:  "server error",
			qname: "error.algo.",
			qtype: dns.TypeA,
			mockHandler: &mockNfdRRHandler{
				err: errors.New("internal error"),
			},
			expectedRcode:  dns.RcodeServerFailure,
			expectedAnswer: 0,
		},
		{
			name:  "unsupported query type",
			qname: "test.algo.",
			qtype: dns.TypeANY,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{},
			},
			expectedRcode:  dns.RcodeNotImplemented,
			expectedAnswer: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nfdPlugin := &NfdPlugin{
				NfdHandler: tt.mockHandler,
			}

			req := new(dns.Msg)
			req.SetQuestion(tt.qname, tt.qtype)

			w := &testResponseWriter{}
			rcode, _ := nfdPlugin.ServeDNS(context.Background(), w, req)

			assert.Equal(t, tt.expectedRcode, rcode)
			if w.msg != nil && tt.expectedAnswer > 0 {
				assert.Len(t, w.msg.Answer, tt.expectedAnswer)
			}
		})
	}
}

// TestServeDNSNoDataRewritesBack verifies that a NoData (NFD-not-found) response
// against a *.algo qname is rewritten back to the server-block zone form before
// being delegated, so the file plugin can match its embedded zone and produce
// NXDOMAIN+SOA.
func TestServeDNSNoDataRewritesBack(t *testing.T) {
	tests := []struct {
		name           string
		qname          string
		zoneOrigin     string
		expectDelegate string // empty means expect NameError (no delegation), no rewrite
	}{
		{
			name:           "dotalgo.io: notfound.algo rewrites to notfound.dotalgo.io",
			qname:          "notfound.algo.",
			zoneOrigin:     "dotalgo.io.",
			expectDelegate: "notfound.dotalgo.io.",
		},
		{
			name:           "algo.xyz: notfound.algo rewrites to notfound.algo.xyz",
			qname:          "notfound.algo.",
			zoneOrigin:     "algo.xyz.",
			expectDelegate: "notfound.algo.xyz.",
		},
		{
			name:           "deep label: _acme-challenge.trilemma.algo (NFD missing) rewrites to *.dotalgo.io",
			qname:          "_acme-challenge.trilemma.algo.",
			zoneOrigin:     "dotalgo.io.",
			expectDelegate: "_acme-challenge.trilemma.dotalgo.io.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := &mockNextPlugin{}
			nfdPlugin := &NfdPlugin{
				Next: next,
				NfdHandler: &mockNfdRRHandler{
					err: nfd.ErrNfdNotFound,
				},
				zoneOrigin: tt.zoneOrigin,
			}

			req := new(dns.Msg)
			req.SetQuestion(tt.qname, dns.TypeA)
			w := &testResponseWriter{}

			_, err := nfdPlugin.ServeDNS(context.Background(), w, req)
			require.NoError(t, err)
			assert.Equal(t, tt.expectDelegate, next.receivedName, "delegated query should be rewritten back to zone-origin form")
		})
	}
}

// TestServeDNSNoDataApexFallthrough verifies that apex/non-.algo qnames (which
// arrive un-rewritten because the rewrite rule doesn't match the bare zone) are
// delegated as-is so the file plugin can serve their apex records.
func TestServeDNSNoDataApexFallthrough(t *testing.T) {
	next := &mockNextPlugin{}
	nfdPlugin := &NfdPlugin{
		Next:       next,
		NfdHandler: &mockNfdRRHandler{},
		zoneOrigin: "algo.xyz.",
	}

	req := new(dns.Msg)
	req.SetQuestion("algo.xyz.", dns.TypeSOA)
	w := &testResponseWriter{}

	_, err := nfdPlugin.ServeDNS(context.Background(), w, req)
	require.NoError(t, err)
	// Apex qname doesn't end in ".algo.", so no rewrite should happen.
	assert.Equal(t, "algo.xyz.", next.receivedName)
}

// TestServeDNSNameErrorWritesResponse verifies the NameError path writes an
// NXDOMAIN response with the SOA in the authority section, instead of falling
// through to ServerFailure.
func TestServeDNSNameErrorWritesResponse(t *testing.T) {
	soa, err := dns.NewRR("dotalgo.io. 14400 IN SOA ns1.dotalgo.io. hostmaster.dotalgo.io. 1 4h 1h 7d 4h")
	require.NoError(t, err)

	nfdPlugin := &NfdPlugin{
		NfdHandler: &mockNfdRRHandler{
			rrs: []nfd.JsonRr{
				{Name: "trilemma.algo.", Type: "A", RrData: []string{"1.2.3.4"}, Ttl: 300},
			},
		},
		zoneSOA:    soa,
		zoneOrigin: "dotalgo.io.",
	}

	req := new(dns.Msg)
	req.SetQuestion("_acme-challenge.trilemma.algo.", dns.TypeA)
	w := &testResponseWriter{}

	rcode, err := nfdPlugin.ServeDNS(context.Background(), w, req)
	require.NoError(t, err)
	assert.Equal(t, dns.RcodeNameError, rcode)
	require.NotNil(t, w.msg, "should have written a response")
	assert.Equal(t, dns.RcodeNameError, w.msg.Rcode)
	assert.Empty(t, w.msg.Answer)
	require.Len(t, w.msg.Ns, 1, "authority section should contain SOA")
	_, ok := w.msg.Ns[0].(*dns.SOA)
	assert.True(t, ok, "authority should be SOA RR")
}

// TestServeDNSNoDataReturnsNODATA verifies the NODATA path (name exists, type
// doesn't): the response is NOERROR, empty answer, and SOA in authority.
func TestServeDNSNoDataReturnsNODATA(t *testing.T) {
	soa, err := dns.NewRR("dotalgo.io. 14400 IN SOA ns1.dotalgo.io. hostmaster.dotalgo.io. 1 4h 1h 7d 4h")
	require.NoError(t, err)

	nfdPlugin := &NfdPlugin{
		NfdHandler: &mockNfdRRHandler{
			rrs: []nfd.JsonRr{
				{Name: "trilemma.algo.", Type: "A", RrData: []string{"1.2.3.4"}, Ttl: 300},
			},
		},
		zoneSOA:    soa,
		zoneOrigin: "dotalgo.io.",
	}

	req := new(dns.Msg)
	req.SetQuestion("trilemma.algo.", dns.TypeAAAA) // name exists, AAAA doesn't
	w := &testResponseWriter{}

	rcode, err := nfdPlugin.ServeDNS(context.Background(), w, req)
	require.NoError(t, err)
	assert.Equal(t, dns.RcodeSuccess, rcode)
	require.NotNil(t, w.msg)
	assert.Equal(t, dns.RcodeSuccess, w.msg.Rcode)
	assert.Empty(t, w.msg.Answer)
	require.Len(t, w.msg.Ns, 1)
	_, ok := w.msg.Ns[0].(*dns.SOA)
	assert.True(t, ok, "authority should be SOA RR")
}

func TestLookup(t *testing.T) {
	tests := []struct {
		name           string
		qname          string
		qtype          uint16
		mockHandler    *mockNfdRRHandler
		expectedResult Result
		expectedAnswer int
	}{
		{
			name:  "A record lookup success",
			qname: "test.algo.",
			qtype: dns.TypeA,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "test.algo.", Type: "A", RrData: []string{"192.168.1.1"}, Ttl: 300},
				},
			},
			expectedResult: Success,
			expectedAnswer: 1,
		},
		{
			name:  "AAAA record lookup success",
			qname: "test.algo.",
			qtype: dns.TypeAAAA,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "test.algo.", Type: "AAAA", RrData: []string{"2001:db8::1"}, Ttl: 300},
				},
			},
			expectedResult: Success,
			expectedAnswer: 1,
		},
		{
			name:  "MX record lookup success",
			qname: "test.algo.",
			qtype: dns.TypeMX,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "test.algo.", Type: "MX", RrData: []string{"10 mail.test.algo."}, Ttl: 300},
				},
			},
			expectedResult: Success,
			expectedAnswer: 1,
		},
		{
			name:  "TXT record lookup success",
			qname: "test.algo.",
			qtype: dns.TypeTXT,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "test.algo.", Type: "TXT", RrData: []string{"\"v=spf1 ~all\""}, Ttl: 300},
				},
			},
			expectedResult: Success,
			expectedAnswer: 1,
		},
		{
			// NFD fetched successfully but has no records at all -> name doesn't
			// exist in this NFD's zone -> NXDOMAIN (RFC 1034/2308).
			name:  "NFD has zero records returns NXDOMAIN",
			qname: "test.algo.",
			qtype: dns.TypeA,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{},
			},
			expectedResult: NameError,
			expectedAnswer: 0,
		},
		{
			// NFD fetched successfully, qname has no records of any type
			// (records exist for a sibling name) -> NXDOMAIN.
			name:  "NFD has records but qname missing returns NXDOMAIN",
			qname: "_acme-challenge.trilemma.algo.",
			qtype: dns.TypeA,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "trilemma.algo.", Type: "A", RrData: []string{"1.2.3.4"}, Ttl: 300},
				},
			},
			expectedResult: NameError,
			expectedAnswer: 0,
		},
		{
			// NFD fetched successfully, qname exists but with a different type
			// (A record exists, AAAA queried) -> NODATA (Success with empty answer).
			name:  "NFD has different type returns NODATA",
			qname: "trilemma.algo.",
			qtype: dns.TypeAAAA,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "trilemma.algo.", Type: "A", RrData: []string{"1.2.3.4"}, Ttl: 300},
				},
			},
			expectedResult: Success,
			expectedAnswer: 0,
		},
		{
			name:  "NFD not found",
			qname: "notfound.algo.",
			qtype: dns.TypeA,
			mockHandler: &mockNfdRRHandler{
				err: nfd.ErrNfdNotFound,
			},
			expectedResult: NoData,
			expectedAnswer: 0,
		},
		{
			name:  "server failure",
			qname: "error.algo.",
			qtype: dns.TypeA,
			mockHandler: &mockNfdRRHandler{
				err: errors.New("internal error"),
			},
			expectedResult: ServerFailure,
			expectedAnswer: 0,
		},
		{
			name:  "subdomain A record lookup - bare label",
			qname: "grafana.corvid.algo.",
			qtype: dns.TypeA,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "corvid.algo.", Type: "a", RrData: []string{"72.60.148.52"}, Ttl: 3600},
					{Name: "grafana.corvid.algo.", Type: "a", RrData: []string{"72.60.148.52"}, Ttl: 3600},
				},
			},
			expectedResult: Success,
			expectedAnswer: 1,
		},
		{
			name:  "root A record lookup - corvid.algo",
			qname: "corvid.algo.",
			qtype: dns.TypeA,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "corvid.algo.", Type: "a", RrData: []string{"72.60.148.52"}, Ttl: 3600},
					{Name: "grafana.corvid.algo.", Type: "a", RrData: []string{"72.60.148.52"}, Ttl: 3600},
				},
			},
			expectedResult: Success,
			expectedAnswer: 1,
		},
		{
			name:  "root AAAA record lookup - corvid.algo",
			qname: "corvid.algo.",
			qtype: dns.TypeAAAA,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "corvid.algo.", Type: "aaaa", RrData: []string{"2a02:4780:66:5c13::1"}, Ttl: 3600},
				},
			},
			expectedResult: Success,
			expectedAnswer: 1,
		},
		{
			name:  "root MX record lookup - corvid.algo",
			qname: "corvid.algo.",
			qtype: dns.TypeMX,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "corvid.algo.", Type: "mx", RrData: []string{"10 mail.protonmail.ch.", "20 mailsec.protonmail.ch."}, Ttl: 3600},
				},
			},
			expectedResult: Success,
			expectedAnswer: 2,
		},
		{
			name:           "root zone subdomain _psl bypasses NFD lookup",
			qname:          "_psl.algo.",
			qtype:          dns.TypeTXT,
			mockHandler:    &mockNfdRRHandler{},
			expectedResult: Delegation,
			expectedAnswer: 0,
		},
		{
			name:  "underscore segment under NFD still does NFD lookup",
			qname: "_atproto.patrick.algo.",
			qtype: dns.TypeTXT,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "_atproto.patrick.algo.", Type: "TXT", RrData: []string{"did=did:plc:example"}, Ttl: 300},
				},
			},
			expectedResult: Success,
			expectedAnswer: 1,
		},
		{
			name:  "SRV-style two-label underscore prefix routes to NFD lookup",
			qname: "_test._tcp.foo.algo.",
			qtype: dns.TypeSRV,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "_test._tcp.foo.algo.", Type: "SRV", RrData: []string{"10 5 8883 broker.foo.algo."}, Ttl: 300},
				},
			},
			expectedResult: Success,
			expectedAnswer: 1,
		},
		{
			name:  "root CAA record lookup - corvid.algo",
			qname: "corvid.algo.",
			qtype: dns.TypeCAA,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "corvid.algo.", Type: "caa", RrData: []string{"0 issue \"letsencrypt.org\""}, Ttl: 3600},
				},
			},
			expectedResult: Success,
			expectedAnswer: 1,
		},
		{
			// NS query for an existing NFD apex returns NODATA (Success with
			// empty answer). NFDs are not delegated subzones — they cannot
			// define their own NS records — but the apex name *exists*, so
			// the right answer is NOERROR/NODATA, not NXDOMAIN.
			name:  "NS query on existing NFD apex returns NODATA",
			qname: "trilemma.algo.",
			qtype: dns.TypeNS,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "trilemma.algo.", Type: "A", RrData: []string{"1.2.3.4"}, Ttl: 300},
				},
			},
			expectedResult: Success,
			expectedAnswer: 0,
		},
		{
			// NS query for a name that doesn't exist anywhere in the NFD's
			// records is NXDOMAIN — same NXDOMAIN/NODATA decision as any
			// other qtype. Previously this returned NODATA unconditionally,
			// which was inconsistent with the per-record check used for
			// other types.
			name:  "NS query on non-existent NFD subname returns NXDOMAIN",
			qname: "nonexistent.trilemma.algo.",
			qtype: dns.TypeNS,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "trilemma.algo.", Type: "A", RrData: []string{"1.2.3.4"}, Ttl: 300},
				},
			},
			expectedResult: NameError,
			expectedAnswer: 0,
		},
		{
			// NS query for an existing subname (different type) returns NODATA.
			name:  "NS query on existing NFD subname returns NODATA",
			qname: "www.trilemma.algo.",
			qtype: dns.TypeNS,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "www.trilemma.algo.", Type: "A", RrData: []string{"1.2.3.4"}, Ttl: 300},
				},
			},
			expectedResult: Success,
			expectedAnswer: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nfdPlugin := &NfdPlugin{
				NfdHandler: tt.mockHandler,
			}

			req := new(dns.Msg)
			req.SetQuestion(tt.qname, tt.qtype)

			w := &testResponseWriter{}
			state := request.Request{W: w, Req: req}

			answer, _, _, result := nfdPlugin.Lookup(context.Background(), state)

			assert.Equal(t, tt.expectedResult, result)
			assert.Len(t, answer, tt.expectedAnswer)
		})
	}
}

func TestQuery(t *testing.T) {
	nfdPlugin := &NfdPlugin{}

	tests := []struct {
		name          string
		jsonRecords   []nfd.JsonRr
		queryName     string
		qType         uint16
		expectedCount int
		expectNotImpl bool
	}{
		{
			name: "filter A records",
			jsonRecords: []nfd.JsonRr{
				{Name: "test.algo.", Type: "A", RrData: []string{"192.168.1.1"}, Ttl: 300},
				{Name: "test.algo.", Type: "AAAA", RrData: []string{"2001:db8::1"}, Ttl: 300},
			},
			queryName:     "test.algo.",
			qType:         dns.TypeA,
			expectedCount: 1,
		},
		{
			name: "filter AAAA records",
			jsonRecords: []nfd.JsonRr{
				{Name: "test.algo.", Type: "A", RrData: []string{"192.168.1.1"}, Ttl: 300},
				{Name: "test.algo.", Type: "AAAA", RrData: []string{"2001:db8::1"}, Ttl: 300},
			},
			queryName:     "test.algo.",
			qType:         dns.TypeAAAA,
			expectedCount: 1,
		},
		{
			name: "filter by name",
			jsonRecords: []nfd.JsonRr{
				{Name: "test.algo.", Type: "A", RrData: []string{"192.168.1.1"}, Ttl: 300},
				{Name: "other.algo.", Type: "A", RrData: []string{"192.168.1.2"}, Ttl: 300},
			},
			queryName:     "test.algo.",
			qType:         dns.TypeA,
			expectedCount: 1,
		},
		{
			name: "unsupported type returns not implemented",
			jsonRecords: []nfd.JsonRr{
				{Name: "test.algo.", Type: "A", RrData: []string{"192.168.1.1"}, Ttl: 300},
			},
			queryName:     "test.algo.",
			qType:         dns.TypeANY,
			expectNotImpl: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rrs, err := nfdPlugin.Query(tt.jsonRecords, tt.queryName, tt.qType)

			if tt.expectNotImpl {
				assert.ErrorIs(t, err, errNotImplemented)
				return
			}

			require.NoError(t, err)
			assert.Len(t, rrs, tt.expectedCount)
		})
	}
}

func TestCNAMERecursion(t *testing.T) {
	tests := []struct {
		name            string
		qname           string
		qtype           uint16
		mockHandler     *mockNfdRRHandler
		expectedResult  Result
		expectedAnswers int
		validateAnswers func(t *testing.T, answers []dns.RR)
	}{
		{
			name:  "CNAME with successful resolution",
			qname: "www.test.algo.",
			qtype: dns.TypeA,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "www.test.algo.", Type: "CNAME", RrData: []string{"test.algo."}, Ttl: 300},
					{Name: "test.algo.", Type: "A", RrData: []string{"192.168.1.1"}, Ttl: 300},
				},
			},
			expectedResult:  Success,
			expectedAnswers: 2, // CNAME + A record
			validateAnswers: func(t *testing.T, answers []dns.RR) {
				// First should be CNAME
				cname, ok := answers[0].(*dns.CNAME)
				require.True(t, ok, "first answer should be CNAME")
				assert.Equal(t, "test.algo.", cname.Target)

				// Second should be A record
				a, ok := answers[1].(*dns.A)
				require.True(t, ok, "second answer should be A")
				assert.Equal(t, "192.168.1.1", a.A.String())
			},
		},
		{
			name:  "CNAME query returns only CNAME",
			qname: "www.test.algo.",
			qtype: dns.TypeCNAME,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "www.test.algo.", Type: "CNAME", RrData: []string{"test.algo."}, Ttl: 300},
					{Name: "test.algo.", Type: "A", RrData: []string{"192.168.1.1"}, Ttl: 300},
				},
			},
			expectedResult:  Success,
			expectedAnswers: 1, // Only CNAME
		},
		{
			name:  "CNAME with no A record returns CNAME only",
			qname: "www.test.algo.",
			qtype: dns.TypeA,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{
					{Name: "www.test.algo.", Type: "CNAME", RrData: []string{"external.example.com."}, Ttl: 300},
				},
			},
			expectedResult:  Success, // Per RFC 1034, return CNAME even if target not resolved
			expectedAnswers: 1,       // Just the CNAME
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nfdPlugin := &NfdPlugin{
				NfdHandler: tt.mockHandler,
				Forwarder: &mockForwarder{
					rcode: dns.RcodeServerFailure, // Simulate forwarder failure for external domains
				},
			}

			req := new(dns.Msg)
			req.SetQuestion(tt.qname, tt.qtype)

			w := &testResponseWriter{}
			state := request.Request{W: w, Req: req}

			answer, _, _, result := nfdPlugin.Lookup(context.Background(), state)

			assert.Equal(t, tt.expectedResult, result)
			assert.Len(t, answer, tt.expectedAnswers)

			if tt.validateAnswers != nil {
				tt.validateAnswers(t, answer)
			}
		})
	}
}

func TestPluginName(t *testing.T) {
	p := &NfdPlugin{}
	assert.Equal(t, "nfd", p.Name())
}

func TestResultConstants(t *testing.T) {
	// Verify result constants are defined and distinct
	results := []Result{Success, NameError, Delegation, NoData, ServerFailure, NotImplemented}
	seen := make(map[Result]bool)
	for _, r := range results {
		assert.False(t, seen[r], "duplicate result constant")
		seen[r] = true
	}
	assert.Len(t, seen, 6, "should have 6 distinct result constants")
}

func TestLookupWithShortDomain(t *testing.T) {
	// Test that short domains (less than 2 parts) trigger forwarder
	mockFwd := &mockForwarder{
		answer: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{Name: "example.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   net.ParseIP("1.2.3.4"),
			},
		},
		rcode: dns.RcodeSuccess,
	}

	nfdPlugin := &NfdPlugin{
		Forwarder:  mockFwd,
		NfdHandler: &mockNfdRRHandler{},
	}

	req := new(dns.Msg)
	req.SetQuestion("example.", dns.TypeA)

	w := &testResponseWriter{}
	state := request.Request{W: w, Req: req}

	answer, _, _, result := nfdPlugin.Lookup(context.Background(), state)

	assert.Equal(t, Success, result)
	assert.Len(t, answer, 1)
}

func TestServeDNSDelegation(t *testing.T) {
	tests := []struct {
		name            string
		qname           string
		qtype           uint16
		zoneOrigin      string
		expectedRewrite string
		expectedRcode   int
	}{
		{
			name:            "delegation rewrites _psl.algo to _psl.algo.xyz",
			qname:           "_psl.algo.",
			qtype:           dns.TypeTXT,
			zoneOrigin:      "algo.xyz.",
			expectedRewrite: "_psl.algo.xyz.",
			expectedRcode:   dns.RcodeSuccess,
		},
		{
			name:            "delegation rewrites _dmarc.algo to _dmarc.algo.xyz",
			qname:           "_dmarc.algo.",
			qtype:           dns.TypeTXT,
			zoneOrigin:      "algo.xyz.",
			expectedRewrite: "_dmarc.algo.xyz.",
			expectedRcode:   dns.RcodeSuccess,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockNext := &mockNextPlugin{}

			nfdPlugin := &NfdPlugin{
				Next:       mockNext,
				NfdHandler: &mockNfdRRHandler{},
				zoneOrigin: tt.zoneOrigin,
			}

			req := new(dns.Msg)
			req.SetQuestion(tt.qname, tt.qtype)

			w := &testResponseWriter{}
			rcode, _ := nfdPlugin.ServeDNS(context.Background(), w, req)

			assert.Equal(t, tt.expectedRcode, rcode)
			assert.Equal(t, tt.expectedRewrite, mockNext.receivedName,
				"Next plugin should receive rewritten query name")
		})
	}
}

func TestLookupNonAlgoDomain(t *testing.T) {
	// Test that non-.algo domains are handled correctly
	tests := []struct {
		name           string
		qname          string
		hasNext        bool
		expectedResult Result
	}{
		{
			name:           "algo.xyz root zone - delegate to file plugin",
			qname:          "algo.xyz.",
			hasNext:        false,
			expectedResult: NoData,
		},
		{
			name:           "dotalgo.io root zone - delegate to file plugin",
			qname:          "dotalgo.io.",
			hasNext:        false,
			expectedResult: NoData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var next plugin.Handler
			if tt.hasNext {
				next = &mockForwarder{rcode: dns.RcodeSuccess}
			}

			nfdPlugin := &NfdPlugin{
				Next:       next,
				Forwarder:  &mockForwarder{rcode: dns.RcodeSuccess},
				NfdHandler: &mockNfdRRHandler{},
			}

			req := new(dns.Msg)
			req.SetQuestion(tt.qname, dns.TypeA)

			w := &testResponseWriter{}
			state := request.Request{W: w, Req: req}

			_, _, _, result := nfdPlugin.Lookup(context.Background(), state)

			assert.Equal(t, tt.expectedResult, result)
		})
	}
}
