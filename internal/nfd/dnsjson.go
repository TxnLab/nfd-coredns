/*
 * Copyright (c) 2024-2026. TxnLab Inc.
 * All Rights reserved.
 */

package nfd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/miekg/dns"
)

var (
	ErrInvalidDNSJson = fmt.Errorf("invalid DNS json")
)

const (
	minTTL     = 60    // 1 minute minimum
	maxTTL     = 86400 // 24 hours maximum
	defaultTTL = 300   // 5 minutes default
)

type JsonRRs struct {
	Rrs []JsonRr `json:"rr"`
}

// JsonRr represents a DNS resource record in JSON format with name, type, TTL, and record-specific data.
type JsonRr struct {
	// Name will be like @ (for origin - ie: patrick.algo.xyz., or a name like box which would
	// represent box.patrick.algo.xyz
	Name   string   `json:"name"`
	RrData []string `json:"rrData"`
	Ttl    int      `json:"ttl,omitempty"`
	Type   string   `json:"type"`
}

// nfdToJsonRRs converts NFD properties to a slice of JsonRr records, processing DNS json data and merging in
// Bluesky data if available.
// It returns an error if DNS data unmarshalling fails.
func nfdToJsonRRs(_ context.Context, nfdProps Properties) ([]JsonRr, error) {
	dnsVal, found := nfdProps.UserDefined["dns"]
	var dnsResult []JsonRr
	if found {
		// unmarshal into dnsResult
		err := json.Unmarshal([]byte(dnsVal), &dnsResult)
		if err != nil {
			// return nil, errors.Wrapf(ErrInvalidDNSJson, "failed to unmarshal dns property: %v", err)
			return nil, fmt.Errorf("failed to unmarshal dns property: %w", ErrInvalidDNSJson)
		}
	}
	// Mix in bluesky record if appropriate
	if bskydid, found := nfdProps.Verified["blueskydid"]; found {
		dnsResult = append(dnsResult, JsonRr{
			Name:   "_atproto.@",
			Type:   "txt",
			RrData: []string{"did=" + bskydid},
		})
	}
	return dnsResult, nil
}

// NameExistsInJsonRRs reports whether any record (any type) matches the given name,
// using the same case-insensitive comparison as DnsRRsFromJsonRRs. Used to distinguish
// NXDOMAIN (no records of any type for the name) from NODATA (name exists, type doesn't).
func NameExistsInJsonRRs(jsonRecords []JsonRr, queryName string) bool {
	for _, jsonRecord := range jsonRecords {
		if strings.EqualFold(jsonRecord.Name, queryName) {
			return true
		}
	}
	return false
}

// DnsRRsFromJsonRRs returns RR's that match the given name and type (from pre-merged root/segment data)
func DnsRRsFromJsonRRs(jsonRecords []JsonRr, queryName string, rrType uint16) ([]dns.RR, error) {
	var (
		rrs = make([]dns.RR, 0, len(jsonRecords))
	)

	typeName, found := dns.TypeToString[rrType]
	if !found {
		return nil, fmt.Errorf("failed to find type name for %d", rrType)
	}
	for _, jsonRecord := range jsonRecords {
		if !strings.EqualFold(jsonRecord.Type, typeName) || !strings.EqualFold(jsonRecord.Name, queryName) {
			continue
		}
		// compose as dns string for parsing
		// ie: json of:
		// {
		//  "name": "example.com.",
		//  "rrData": [
		//      "10 mail.example.com.",
		//      "20 mail2.example.com."
		//  ],
		//  "ttl": 86400,
		//  "type": "MX"
		// }
		// would get converted to not one, but two records, using the same values except for the rrdatas at the end
		// example.com. 86400 IN MX 10 mail.example.com.
		// example.com. 86400 IN MX 20 mail2.example.com.
		ttl := defaultTTL
		if jsonRecord.Ttl > 0 {
			ttl = min(max(jsonRecord.Ttl, minTTL), maxTTL)
		}
		for _, rrdata := range jsonRecord.RrData {
			dnsString := jsonRecord.Name + " " + strconv.Itoa(ttl) + " " + dns.ClassToString[dns.ClassINET] + " " + jsonRecord.Type + " "
			dnsString += rrdata
			rr, err := dns.NewRR(dnsString)
			if err != nil {
				return nil, fmt.Errorf("failed to parse dns string: %s", dnsString)
			}
			rrs = append(rrs, rr)
		}
	}
	return rrs, nil
}

// ConvertOriginRefs rewrites the Name field of each RR so that every record is
// rooted under the NFD's own FQDN. An NFD has DNS authority only over its own
// subtree (the NFD FQDN itself and names ending in ".<fqdn>."); anything else,
// including cross-root references like "test.bar.algo." stored on foo.algo, is
// unservable as written and is re-rooted under the NFD as a relative subname.
//
// Accepted name forms that resolve as canonical, in-scope records:
//   - "@"                       → the NFD itself (e.g., foo.algo.)
//   - "<sub>.@"                 → "<sub>.<fqdn>." (e.g., _test._tcp.foo.algo.)
//   - "<sub>.<fqdn>[.]"         → kept as canonical FQDN
//   - "<sub>.<fqdn-mirror>[.]"  → mirror normalized to canonical form
//
// where <fqdn-mirror> is the .algo.xyz or .dotalgo.io alias of <fqdn>. The
// trailing dot is optional in all cases and added if missing.
//
// Anything else is treated as a relative subname under the NFD: bare labels
// ("www"), trailing-dot footguns at the DNS root ("_test._tcp."), and
// cross-root references ("test.bar.algo." stored on foo.algo). Any trailing
// dot on the original name is stripped before re-rooting.
func ConvertOriginRefs(_ context.Context, fqdn string, rrs []JsonRr) {
	nfdFqdn := dns.Fqdn(fqdn)     // e.g., "foo.algo."
	nfdSubSuffix := "." + nfdFqdn // e.g., ".foo.algo."
	for i, rr := range rrs {
		name := rr.Name
		if name == "@" {
			rrs[i].Name = nfdFqdn
			continue
		}
		// Replace the '.@' origin marker with the NFD's FQDN.
		if strings.HasSuffix(name, ".@") {
			name = name[:len(name)-1] + nfdFqdn
		}
		// Normalize legacy/alt-mirror suffixes (.algo.xyz, .dotalgo.io) to the
		// canonical .algo form, in both with-trailing-dot and bare forms.
		switch {
		case strings.HasSuffix(name, ".algo.xyz."):
			name = strings.TrimSuffix(name, "xyz.") // xxx.algo.xyz. -> xxx.algo.
		case strings.HasSuffix(name, ".algo.xyz"):
			name = strings.TrimSuffix(name, "xyz") // xxx.algo.xyz -> xxx.algo.
		case strings.HasSuffix(name, ".dotalgo.io."):
			name = strings.TrimSuffix(name, ".dotalgo.io.") + ".algo."
		case strings.HasSuffix(name, ".dotalgo.io"):
			name = strings.TrimSuffix(name, ".dotalgo.io") + ".algo."
		}
		// Bring bare ".algo" forms into FQDN form so the scope check below is
		// comparing apples to apples.
		if strings.HasSuffix(name, ".algo") {
			name += "."
		}
		// Scope enforcement: the resulting name MUST lie inside the NFD's own
		// FQDN subtree. Anything outside is re-rooted under the NFD as a
		// relative subname. This catches:
		//   - bare labels (e.g., "www")
		//   - trailing-dot forms at the DNS root (e.g., "_test._tcp.")
		//   - cross-root references (e.g., "test.bar.algo." stored on foo.algo)
		// The NFD owner has no DNS authority outside their own zone, so we
		// must not emit authoritative records for anything else.
		if name != nfdFqdn && !strings.HasSuffix(name, nfdSubSuffix) {
			name = strings.TrimSuffix(name, ".") + nfdSubSuffix
		}
		rrs[i].Name = name
	}
}

func MergeJsonRrrs(_ context.Context, base []JsonRr, segment []JsonRr) []JsonRr {
	// start with base data, then add entries from segment ONLY if base doesn't have the same name and type
	// in any of its records
	var ret = base
	for _, segmentRecord := range segment {
		found := false
		for _, baseRecord := range ret {
			if strings.EqualFold(baseRecord.Name, segmentRecord.Name) && strings.EqualFold(baseRecord.Type, segmentRecord.Type) {
				found = true
				break
			}
		}
		if !found {
			ret = append(ret, segmentRecord)
		}
	}

	return ret
}
