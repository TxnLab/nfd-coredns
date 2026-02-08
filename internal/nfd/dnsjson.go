/*
 * Copyright (c) 2024. TxnLab Inc.
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

func ConvertOriginRefs(_ context.Context, fqdn string, rrs []JsonRr) {
	// walk the rr's and if name is @ - switch out to the fqdn
	for i, rr := range rrs {
		if rr.Name == "@" {
			rrs[i].Name = dns.Fqdn(fqdn)
		} else {
			if strings.HasSuffix(rr.Name, ".@") {
				// convert foo.@ into foo.{domain}
				rrs[i].Name = rr.Name[:len(rr.Name)-1] + dns.Fqdn(fqdn)
			}
			if strings.HasSuffix(rr.Name, ".algo.xyz") {
				// trim off the .xyz to turn into just .algo
				rrs[i].Name = strings.TrimSuffix(rr.Name, "xyz") // xxx.algo.xyz. -> xxx.algo.
			}
			if strings.HasSuffix(rr.Name, ".dotalgo.io") {
				// replace .dotalgo.io with just .algo
				rrs[i].Name = strings.TrimSuffix(rr.Name, ".dotalgo.io") + ".algo."
			}
			if !strings.HasSuffix(rrs[i].Name, ".") {
				if strings.HasSuffix(rrs[i].Name, ".algo") {
					// Already fully qualified (e.g., "www.patrick.algo"), just needs trailing dot
					rrs[i].Name += "."
				} else {
					// Bare/relative label (e.g., "grafana") - make relative to FQDN
					rrs[i].Name = rrs[i].Name + "." + dns.Fqdn(fqdn)
				}
			}
		}
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
