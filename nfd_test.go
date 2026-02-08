/*
 * Copyright (c) 2025. TxnLab Inc.
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
			name:  "no records found",
			qname: "test.algo.",
			qtype: dns.TypeA,
			mockHandler: &mockNfdRRHandler{
				rrs: []nfd.JsonRr{},
			},
			expectedResult: NoData,
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
