/*
 * Copyright (c) 2024-2026. TxnLab Inc.
 * All Rights reserved.
 */

package nfd

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeJsonRrrs(t *testing.T) {
	tests := []struct {
		name    string
		base    []JsonRr
		segment []JsonRr
		want    []JsonRr
	}{
		{
			name: "Non-overlapping entries",
			base: []JsonRr{
				{
					Name:   "example.com.",
					RrData: []string{"foo", "bar"},
					Type:   "A",
				},
			},
			segment: []JsonRr{
				{Name: "example.net.", Type: "A"},
			},
			want: []JsonRr{
				{Name: "example.net.", Type: "A"},
				{Name: "example.com.", Type: "A", RrData: []string{"foo", "bar"}},
			},
		},
		{
			name: "Overlapping entries",
			base: []JsonRr{
				{Name: "example.com.", Type: "A", RrData: []string{"base version"}},
			},
			segment: []JsonRr{
				{Name: "example.com.", Type: "A", RrData: []string{"segment version"}},
				{Name: "example.com.", Type: "A", RrData: []string{"segment version 2"}},
			},
			want: []JsonRr{
				{Name: "example.com.", Type: "A", RrData: []string{"base version"}},
			},
		},
		{
			name: "Multiple overlapping entries",
			base: []JsonRr{
				{Name: "example.com.", Type: "A", RrData: []string{"base com version"}},
				{Name: "example.com.", Type: "A", RrData: []string{"base com version 2"}},
				{Name: "example.net.", Type: "A", RrData: []string{"base net version"}},
			},
			segment: []JsonRr{
				{Name: "example.com.", Type: "A", RrData: []string{"segment com version"}},
				{Name: "example.net.", Type: "A", RrData: []string{"segment net version"}},
			},
			want: []JsonRr{
				{Name: "example.com.", Type: "A", RrData: []string{"base com version"}},
				{Name: "example.com.", Type: "A", RrData: []string{"base com version 2"}},
				{Name: "example.net.", Type: "A", RrData: []string{"base net version"}},
			},
		},
		{
			name: "Interleaved overlapping entries",
			base: []JsonRr{
				{Name: "example.com.", Type: "A", RrData: []string{"base com version"}},
				{Name: "example.org.", Type: "A", RrData: []string{"base org version"}},
			},
			segment: []JsonRr{
				{Name: "example.org.", Type: "A", RrData: []string{"segment org version"}},
				{Name: "example.net.", Type: "A", RrData: []string{"segment net version"}},
			},
			want: []JsonRr{
				{Name: "example.com.", Type: "A", RrData: []string{"base com version"}},
				{Name: "example.org.", Type: "A", RrData: []string{"base org version"}},
				{Name: "example.net.", Type: "A", RrData: []string{"segment net version"}},
			},
		},
		{
			name: "Base empty",
			base: []JsonRr{},
			segment: []JsonRr{
				{Name: "example.net.", Type: "A"},
			},
			want: []JsonRr{
				{Name: "example.net.", Type: "A"},
			},
		},
		{
			name: "Segment empty",
			base: []JsonRr{
				{Name: "example.com.", Type: "A"},
			},
			segment: []JsonRr{},
			want: []JsonRr{
				{Name: "example.com.", Type: "A"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeJsonRrrs(context.Background(), tt.base, tt.segment)
			if !equal(got, tt.want) {
				t.Errorf("MergeJsonRrrs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func equal(left []JsonRr, right []JsonRr) bool {
	if len(left) != len(right) {
		return false
	}

	// Sort both slices to make sure the order doesn't affect the comparison
	sort.Slice(left, func(i, j int) bool {
		return left[i].Name < left[j].Name
	})
	sort.Slice(right, func(i, j int) bool {
		return right[i].Name < right[j].Name
	})

	for i := range left {
		if left[i].Name != right[i].Name ||
			!reflect.DeepEqual(left[i].RrData, right[i].RrData) ||
			left[i].Ttl != right[i].Ttl ||
			left[i].Type != right[i].Type {
			return false
		}
	}

	return true
}

func TestDnsRRsFromJsonRRs(t *testing.T) {
	tests := []struct {
		name        string
		jsonRecords []JsonRr
		queryName   string
		rrType      uint16
		expectCount int
		expectError bool
		validate    func(t *testing.T, rrs []dns.RR)
	}{
		{
			name: "A record",
			jsonRecords: []JsonRr{
				{Name: "test.algo.", Type: "A", RrData: []string{"192.168.1.1"}, Ttl: 300},
			},
			queryName:   "test.algo.",
			rrType:      dns.TypeA,
			expectCount: 1,
			validate: func(t *testing.T, rrs []dns.RR) {
				a, ok := rrs[0].(*dns.A)
				require.True(t, ok)
				assert.Equal(t, "192.168.1.1", a.A.String())
				assert.Equal(t, uint32(300), a.Hdr.Ttl)
			},
		},
		{
			name: "AAAA record",
			jsonRecords: []JsonRr{
				{Name: "test.algo.", Type: "AAAA", RrData: []string{"2001:db8::1"}, Ttl: 600},
			},
			queryName:   "test.algo.",
			rrType:      dns.TypeAAAA,
			expectCount: 1,
			validate: func(t *testing.T, rrs []dns.RR) {
				aaaa, ok := rrs[0].(*dns.AAAA)
				require.True(t, ok)
				assert.Equal(t, "2001:db8::1", aaaa.AAAA.String())
			},
		},
		{
			name: "MX record",
			jsonRecords: []JsonRr{
				{Name: "test.algo.", Type: "MX", RrData: []string{"10 mail.test.algo."}, Ttl: 3600},
			},
			queryName:   "test.algo.",
			rrType:      dns.TypeMX,
			expectCount: 1,
			validate: func(t *testing.T, rrs []dns.RR) {
				mx, ok := rrs[0].(*dns.MX)
				require.True(t, ok)
				assert.Equal(t, uint16(10), mx.Preference)
				assert.Equal(t, "mail.test.algo.", mx.Mx)
			},
		},
		{
			name: "TXT record",
			jsonRecords: []JsonRr{
				{Name: "test.algo.", Type: "TXT", RrData: []string{"\"v=spf1 include:_spf.google.com ~all\""}, Ttl: 300},
			},
			queryName:   "test.algo.",
			rrType:      dns.TypeTXT,
			expectCount: 1,
			validate: func(t *testing.T, rrs []dns.RR) {
				txt, ok := rrs[0].(*dns.TXT)
				require.True(t, ok)
				assert.Contains(t, txt.Txt, "v=spf1 include:_spf.google.com ~all")
			},
		},
		{
			name: "CNAME record",
			jsonRecords: []JsonRr{
				{Name: "www.test.algo.", Type: "CNAME", RrData: []string{"test.algo."}, Ttl: 300},
			},
			queryName:   "www.test.algo.",
			rrType:      dns.TypeCNAME,
			expectCount: 1,
			validate: func(t *testing.T, rrs []dns.RR) {
				cname, ok := rrs[0].(*dns.CNAME)
				require.True(t, ok)
				assert.Equal(t, "test.algo.", cname.Target)
			},
		},
		{
			name: "SRV record",
			jsonRecords: []JsonRr{
				{Name: "_http._tcp.test.algo.", Type: "SRV", RrData: []string{"10 5 80 web.test.algo."}, Ttl: 300},
			},
			queryName:   "_http._tcp.test.algo.",
			rrType:      dns.TypeSRV,
			expectCount: 1,
			validate: func(t *testing.T, rrs []dns.RR) {
				srv, ok := rrs[0].(*dns.SRV)
				require.True(t, ok)
				assert.Equal(t, uint16(10), srv.Priority)
				assert.Equal(t, uint16(5), srv.Weight)
				assert.Equal(t, uint16(80), srv.Port)
				assert.Equal(t, "web.test.algo.", srv.Target)
			},
		},
		{
			name: "CAA record",
			jsonRecords: []JsonRr{
				{Name: "test.algo.", Type: "CAA", RrData: []string{"0 issue \"letsencrypt.org\""}, Ttl: 3600},
			},
			queryName:   "test.algo.",
			rrType:      dns.TypeCAA,
			expectCount: 1,
			validate: func(t *testing.T, rrs []dns.RR) {
				caa, ok := rrs[0].(*dns.CAA)
				require.True(t, ok)
				assert.Equal(t, "issue", caa.Tag)
				assert.Equal(t, "letsencrypt.org", caa.Value)
			},
		},
		{
			name: "NS record",
			jsonRecords: []JsonRr{
				{Name: "test.algo.", Type: "NS", RrData: []string{"ns1.test.algo."}, Ttl: 3600},
			},
			queryName:   "test.algo.",
			rrType:      dns.TypeNS,
			expectCount: 1,
			validate: func(t *testing.T, rrs []dns.RR) {
				ns, ok := rrs[0].(*dns.NS)
				require.True(t, ok)
				assert.Equal(t, "ns1.test.algo.", ns.Ns)
			},
		},
		{
			name: "multiple A records",
			jsonRecords: []JsonRr{
				{Name: "test.algo.", Type: "A", RrData: []string{"192.168.1.1", "192.168.1.2"}, Ttl: 300},
			},
			queryName:   "test.algo.",
			rrType:      dns.TypeA,
			expectCount: 2,
		},
		{
			name: "no matching records",
			jsonRecords: []JsonRr{
				{Name: "other.algo.", Type: "A", RrData: []string{"192.168.1.1"}, Ttl: 300},
			},
			queryName:   "test.algo.",
			rrType:      dns.TypeA,
			expectCount: 0,
		},
		{
			name: "case insensitive type matching",
			jsonRecords: []JsonRr{
				{Name: "test.algo.", Type: "a", RrData: []string{"192.168.1.1"}, Ttl: 300},
			},
			queryName:   "test.algo.",
			rrType:      dns.TypeA,
			expectCount: 1,
		},
		{
			name: "case insensitive name matching",
			jsonRecords: []JsonRr{
				{Name: "TEST.ALGO.", Type: "A", RrData: []string{"192.168.1.1"}, Ttl: 300},
			},
			queryName:   "test.algo.",
			rrType:      dns.TypeA,
			expectCount: 1,
		},
		{
			name: "default TTL when not specified",
			jsonRecords: []JsonRr{
				{Name: "test.algo.", Type: "A", RrData: []string{"192.168.1.1"}},
			},
			queryName:   "test.algo.",
			rrType:      dns.TypeA,
			expectCount: 1,
			validate: func(t *testing.T, rrs []dns.RR) {
				assert.Equal(t, uint32(defaultTTL), rrs[0].Header().Ttl)
			},
		},
		{
			name: "TTL clamped to minimum",
			jsonRecords: []JsonRr{
				{Name: "test.algo.", Type: "A", RrData: []string{"192.168.1.1"}, Ttl: 10},
			},
			queryName:   "test.algo.",
			rrType:      dns.TypeA,
			expectCount: 1,
			validate: func(t *testing.T, rrs []dns.RR) {
				assert.Equal(t, uint32(minTTL), rrs[0].Header().Ttl)
			},
		},
		{
			name: "TTL clamped to maximum",
			jsonRecords: []JsonRr{
				{Name: "test.algo.", Type: "A", RrData: []string{"192.168.1.1"}, Ttl: 1000000},
			},
			queryName:   "test.algo.",
			rrType:      dns.TypeA,
			expectCount: 1,
			validate: func(t *testing.T, rrs []dns.RR) {
				assert.Equal(t, uint32(maxTTL), rrs[0].Header().Ttl)
			},
		},
		{
			name: "negative TTL uses default",
			jsonRecords: []JsonRr{
				{Name: "test.algo.", Type: "A", RrData: []string{"192.168.1.1"}, Ttl: -100},
			},
			queryName:   "test.algo.",
			rrType:      dns.TypeA,
			expectCount: 1,
			validate: func(t *testing.T, rrs []dns.RR) {
				assert.Equal(t, uint32(defaultTTL), rrs[0].Header().Ttl)
			},
		},
		{
			name: "subdomain A record after bare label conversion",
			jsonRecords: []JsonRr{
				{Name: "grafana.corvid.algo.", Type: "a", RrData: []string{"72.60.148.52"}, Ttl: 3600},
				{Name: "corvid.algo.", Type: "a", RrData: []string{"72.60.148.52"}, Ttl: 3600},
			},
			queryName:   "grafana.corvid.algo.",
			rrType:      dns.TypeA,
			expectCount: 1,
			validate: func(t *testing.T, rrs []dns.RR) {
				a, ok := rrs[0].(*dns.A)
				require.True(t, ok)
				assert.Equal(t, "72.60.148.52", a.A.String())
				assert.Equal(t, uint32(3600), a.Hdr.Ttl)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rrs, err := DnsRRsFromJsonRRs(tt.jsonRecords, tt.queryName, tt.rrType)
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, rrs, tt.expectCount)
			if tt.validate != nil && len(rrs) > 0 {
				tt.validate(t, rrs)
			}
		})
	}
}

func TestConvertOriginRefs(t *testing.T) {
	tests := []struct {
		name     string
		fqdn     string
		input    []JsonRr
		expected []JsonRr
	}{
		{
			name: "@ replaced with fqdn",
			fqdn: "patrick.algo",
			input: []JsonRr{
				{Name: "@"},
			},
			expected: []JsonRr{
				{Name: "patrick.algo."},
			},
		},
		{
			name: "foo.@ replaced with foo.fqdn",
			fqdn: "patrick.algo",
			input: []JsonRr{
				{Name: "www.@"},
			},
			expected: []JsonRr{
				{Name: "www.patrick.algo."},
			},
		},
		{
			name: ".algo.xyz suffix stripped",
			fqdn: "patrick.algo",
			input: []JsonRr{
				{Name: "www.patrick.algo.xyz"},
			},
			expected: []JsonRr{
				{Name: "www.patrick.algo."},
			},
		},
		{
			name: ".dotalgo.io replaced with .algo",
			fqdn: "patrick.algo",
			input: []JsonRr{
				{Name: "www.patrick.dotalgo.io"},
			},
			expected: []JsonRr{
				{Name: "www.patrick.algo."},
			},
		},
		{
			name: "trailing dot added if missing",
			fqdn: "patrick.algo",
			input: []JsonRr{
				{Name: "www.patrick.algo"},
			},
			expected: []JsonRr{
				{Name: "www.patrick.algo."},
			},
		},
		{
			name: "already has trailing dot",
			fqdn: "patrick.algo",
			input: []JsonRr{
				{Name: "www.patrick.algo."},
			},
			expected: []JsonRr{
				{Name: "www.patrick.algo."},
			},
		},
		{
			name: "_atproto.@ for bluesky",
			fqdn: "patrick.algo",
			input: []JsonRr{
				{Name: "_atproto.@"},
			},
			expected: []JsonRr{
				{Name: "_atproto.patrick.algo."},
			},
		},
		{
			name: "multiple records",
			fqdn: "test.algo",
			input: []JsonRr{
				{Name: "@"},
				{Name: "www.@"},
				{Name: "mail.test.algo.xyz"},
			},
			expected: []JsonRr{
				{Name: "test.algo."},
				{Name: "www.test.algo."},
				{Name: "mail.test.algo."},
			},
		},
		{
			name: "bare label converted to FQDN",
			fqdn: "corvid.algo",
			input: []JsonRr{
				{Name: "grafana"},
			},
			expected: []JsonRr{
				{Name: "grafana.corvid.algo."},
			},
		},
		{
			name: "corvid.algo full DNS JSON conversion",
			fqdn: "corvid.algo",
			input: []JsonRr{
				{Name: "@", RrData: []string{"72.60.148.52"}, Type: "a", Ttl: 3600},
				{Name: "www", RrData: []string{"corvid.algo.xyz"}, Type: "cname", Ttl: 3600},
				{Name: "@", RrData: []string{"2a02:4780:66:5c13::1"}, Type: "aaaa", Ttl: 3600},
				{Name: "@", RrData: []string{"protonmail-verification=e17eef2d71c7c838cabe3aece28056e5187be95a"}, Type: "txt", Ttl: 3600},
				{Name: "pera_3225439167.corvid.algo.xyz", RrData: []string{"pera_3225439167_V4mykD2U3k"}, Type: "txt", Ttl: 3600},
				{Name: "@", RrData: []string{"10 mail.protonmail.ch", "20 mailsec.protonmail.ch"}, Type: "mx", Ttl: 3600},
				{Name: "pera_project.corvid.algo.xyz", RrData: []string{"pera_project_vS064sgFss"}, Type: "txt", Ttl: 3600},
				{Name: "@", RrData: []string{"0 issue \"letsencrypt.org\""}, Type: "caa", Ttl: 3600},
				{Name: "bot", RrData: []string{"72.60.148.52"}, Type: "a", Ttl: 3600},
				{Name: "test", RrData: []string{"72.60.148.52"}, Type: "a", Ttl: 3600},
				{Name: "grafana", RrData: []string{"72.60.148.52"}, Type: "a", Ttl: 3600},
				{Name: "jenkins", RrData: []string{"72.60.148.52"}, Type: "a", Ttl: 3600},
			},
			expected: []JsonRr{
				{Name: "corvid.algo.", RrData: []string{"72.60.148.52"}, Type: "a", Ttl: 3600},
				{Name: "www.corvid.algo.", RrData: []string{"corvid.algo.xyz"}, Type: "cname", Ttl: 3600},
				{Name: "corvid.algo.", RrData: []string{"2a02:4780:66:5c13::1"}, Type: "aaaa", Ttl: 3600},
				{Name: "corvid.algo.", RrData: []string{"protonmail-verification=e17eef2d71c7c838cabe3aece28056e5187be95a"}, Type: "txt", Ttl: 3600},
				{Name: "pera_3225439167.corvid.algo.", RrData: []string{"pera_3225439167_V4mykD2U3k"}, Type: "txt", Ttl: 3600},
				{Name: "corvid.algo.", RrData: []string{"10 mail.protonmail.ch", "20 mailsec.protonmail.ch"}, Type: "mx", Ttl: 3600},
				{Name: "pera_project.corvid.algo.", RrData: []string{"pera_project_vS064sgFss"}, Type: "txt", Ttl: 3600},
				{Name: "corvid.algo.", RrData: []string{"0 issue \"letsencrypt.org\""}, Type: "caa", Ttl: 3600},
				{Name: "bot.corvid.algo.", RrData: []string{"72.60.148.52"}, Type: "a", Ttl: 3600},
				{Name: "test.corvid.algo.", RrData: []string{"72.60.148.52"}, Type: "a", Ttl: 3600},
				{Name: "grafana.corvid.algo.", RrData: []string{"72.60.148.52"}, Type: "a", Ttl: 3600},
				{Name: "jenkins.corvid.algo.", RrData: []string{"72.60.148.52"}, Type: "a", Ttl: 3600},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying test data
			input := make([]JsonRr, len(tt.input))
			copy(input, tt.input)

			ConvertOriginRefs(context.Background(), tt.fqdn, input)

			for i, rr := range input {
				assert.Equal(t, tt.expected[i].Name, rr.Name, "record %d name mismatch", i)
			}
		})
	}
}

func TestNfdToJsonRRs(t *testing.T) {
	tests := []struct {
		name        string
		props       Properties
		expectCount int
		expectError bool
		validate    func(t *testing.T, rrs []JsonRr)
	}{
		{
			name: "no dns or bluesky data",
			props: Properties{
				UserDefined: map[string]string{},
				Verified:    map[string]string{},
			},
			expectCount: 0,
		},
		{
			name: "valid dns json",
			props: Properties{
				UserDefined: map[string]string{
					"dns": `[{"name":"@","type":"A","rrData":["192.168.1.1"],"ttl":300}]`,
				},
				Verified: map[string]string{},
			},
			expectCount: 1,
			validate: func(t *testing.T, rrs []JsonRr) {
				assert.Equal(t, "@", rrs[0].Name)
				assert.Equal(t, "A", rrs[0].Type)
			},
		},
		{
			name: "bluesky DID injection",
			props: Properties{
				UserDefined: map[string]string{},
				Verified: map[string]string{
					"blueskydid": "did:plc:abc123",
				},
			},
			expectCount: 1,
			validate: func(t *testing.T, rrs []JsonRr) {
				assert.Equal(t, "_atproto.@", rrs[0].Name)
				assert.Equal(t, "txt", rrs[0].Type)
				assert.Contains(t, rrs[0].RrData[0], "did=did:plc:abc123")
			},
		},
		{
			name: "dns and bluesky combined",
			props: Properties{
				UserDefined: map[string]string{
					"dns": `[{"name":"@","type":"A","rrData":["192.168.1.1"],"ttl":300}]`,
				},
				Verified: map[string]string{
					"blueskydid": "did:plc:xyz789",
				},
			},
			expectCount: 2,
			validate: func(t *testing.T, rrs []JsonRr) {
				// Should have A record and TXT record for bluesky
				hasA := false
				hasTxt := false
				for _, rr := range rrs {
					if rr.Type == "A" {
						hasA = true
					}
					if rr.Type == "txt" && rr.Name == "_atproto.@" {
						hasTxt = true
					}
				}
				assert.True(t, hasA, "expected A record")
				assert.True(t, hasTxt, "expected TXT record for bluesky")
			},
		},
		{
			name: "invalid dns json",
			props: Properties{
				UserDefined: map[string]string{
					"dns": `not valid json`,
				},
				Verified: map[string]string{},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rrs, err := nfdToJsonRRs(context.Background(), tt.props)
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, rrs, tt.expectCount)
			if tt.validate != nil && len(rrs) > 0 {
				tt.validate(t, rrs)
			}
		})
	}
}
