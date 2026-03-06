/*
 * Copyright (c) 2025-2026. TxnLab Inc.
 * All Rights reserved.
 */

package nfd

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/crypto"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/stretchr/testify/assert"

	clog "github.com/coredns/coredns/plugin/pkg/log"
)

func TestGetNfdRRs(t *testing.T) {
	tests := []struct {
		name          string
		qname         string
		nfdRRHandler  func(handler *nfdRRHandler)
		expectedError error
		expectedErrIs error
		expectedRRs   []JsonRr
	}{
		{
			name:  "cache hit",
			qname: "test.algo.",
			nfdRRHandler: func(handler *nfdRRHandler) {
				handler.rrCache.Add("test.algo.", []JsonRr{{Name: "cached_rr"}})
			},
			expectedError: nil,
			expectedRRs:   []JsonRr{{Name: "cached_rr"}},
		},
		{
			name:  "invalid root domain",
			qname: "test.com.",
			nfdRRHandler: func(handler *nfdRRHandler) {
				// No cache setup
			},
			expectedError: errors.New("qnameSplit[len(qnameSplit)-1] != algo"),
			expectedRRs:   nil,
		},
		{
			name:  "too many segments",
			qname: "sub.sub2.sub3.sub4.algo.",
			nfdRRHandler: func(handler *nfdRRHandler) {
				// No cache setup
			},
			expectedError: ErrNfdTooManySegments,
			expectedRRs:   nil,
		},
		{
			name:  "nfd not found",
			qname: "nonexistent.algo.",
			nfdRRHandler: func(handler *nfdRRHandler) {
				handler.nfdFetcher = &mockNfdFetcher{
					fetchFunc: func(ctx context.Context, log clog.P, names []string) (map[string]Properties, error) {
						return nil, ErrNfdNotFound
					},
				}
			},
			expectedError: ErrNfdNotFound,
			expectedRRs:   nil,
		},
		{
			name:  "root domain mismatch",
			qname: "test.algo.",
			nfdRRHandler: func(handler *nfdRRHandler) {
				handler.nfdFetcher = &mockNfdFetcher{
					fetchFunc: func(ctx context.Context, log clog.P, names []string) (map[string]Properties, error) {
						return map[string]Properties{
							"test.algo": {
								Internal:    map[string]string{"name": "mismatch.algo"}, // Does not match the required domain
								UserDefined: map[string]string{"dns": "valid"},
							},
						}, nil
					},
				}
			},
			expectedError: errors.New("nfdRootData.Internal.name: mismatch.algo != test.algo"),
			expectedRRs:   nil,
		},
		{
			name:  "successful fetch but bad json",
			qname: "test.algo.",
			nfdRRHandler: func(handler *nfdRRHandler) {
				handler.nfdFetcher = &mockNfdFetcher{
					fetchFunc: func(ctx context.Context, log clog.P, names []string) (map[string]Properties, error) {
						dummyAccount := crypto.GenerateAccount()
						return map[string]Properties{
							"test.algo": {
								Internal:    map[string]string{"name": "test.algo", "owner": dummyAccount.Address.String()},
								UserDefined: map[string]string{"dns": "bad json"},
							},
						}, nil
					},
				}
			},
			expectedErrIs: ErrInvalidDNSJson,
			expectedRRs:   nil,
		},
		{
			name:  "successful fetch with no segments",
			qname: "test.algo.",
			nfdRRHandler: func(handler *nfdRRHandler) {
				handler.nfdFetcher = &mockNfdFetcher{
					fetchFunc: func(ctx context.Context, log clog.P, names []string) (map[string]Properties, error) {
						dummyAccount := crypto.GenerateAccount()
						dnsData := []JsonRr{
							{
								Name: "test.algo.",
								RrData: []string{
									"127.0.0.1",
								},
								Ttl:  300,
								Type: "A",
							},
						}
						dnsJson, _ := json.Marshal(dnsData)

						return map[string]Properties{
							"test.algo": {
								Internal:    map[string]string{"name": "test.algo", "owner": dummyAccount.Address.String()},
								UserDefined: map[string]string{"dns": string(dnsJson)},
							},
						}, nil
					},
				}
			},
			expectedError: nil,
			expectedRRs: []JsonRr{{
				Name:   "test.algo.",
				RrData: []string{"127.0.0.1"},
				Ttl:    300,
				Type:   "A",
			}}, // Expect some transformed value
		},
		{
			name:  "successful fetch with segments",
			qname: "segment.test.algo.",
			nfdRRHandler: func(handler *nfdRRHandler) {
				handler.nfdFetcher = &mockNfdFetcher{
					fetchFunc: func(ctx context.Context, log clog.P, names []string) (map[string]Properties, error) {
						rootDns := []JsonRr{
							{
								Name: "test.algo.",
								RrData: []string{
									"127.0.0.1",
								},
								Ttl:  300,
								Type: "A",
							},
						}
						rootJson, _ := json.Marshal(rootDns)
						segmentDns := []JsonRr{
							{
								Name: "segment.test.algo.",
								RrData: []string{
									"10.0.0.1",
								},
								Ttl:  300,
								Type: "A",
							},
						}
						segentJson, _ := json.Marshal(segmentDns)

						return map[string]Properties{
							"test.algo": {
								Internal:    map[string]string{"name": "test.algo", "owner": "owner1"},
								UserDefined: map[string]string{"dns": string(rootJson)},
							},
							"segment.test.algo": {
								Internal:    map[string]string{"name": "segment.test.algo", "owner": "owner1"},
								UserDefined: map[string]string{"dns": string(segentJson)},
							},
						}, nil
					},
				}
			},
			expectedError: nil,
			// expect merged values
			expectedRRs: []JsonRr{
				{
					Name:   "test.algo.",
					RrData: []string{"127.0.0.1"},
					Ttl:    300,
					Type:   "A",
				},
				{
					Name:   "segment.test.algo.",
					RrData: []string{"10.0.0.1"},
					Ttl:    300,
					Type:   "A",
				},
			},
		},
		{
			// query a segment that has different owner than root
			name:  "split ownership error in segments",
			qname: "segment.test.algo.",
			nfdRRHandler: func(handler *nfdRRHandler) {
				handler.nfdFetcher = &mockNfdFetcher{
					fetchFunc: func(ctx context.Context, log clog.P, names []string) (map[string]Properties, error) {
						// still need 'valid' dns just to get past that check - don't worry about contents
						dnsData := []JsonRr{
							{
								Name: "test.algo.",
								RrData: []string{
									"127.0.0.1",
								},
								Ttl:  300,
								Type: "A",
							},
						}
						dnsJson, _ := json.Marshal(dnsData)
						return map[string]Properties{
							"test.algo": {
								Internal:    map[string]string{"name": "test.algo", "owner": "owner1"},
								UserDefined: map[string]string{"dns": string(dnsJson)},
							},
							"segment.test.algo": {
								Internal:    map[string]string{"name": "segment.test.algo", "owner": "owner2"}, // Different owner
								UserDefined: map[string]string{"dns": string(dnsJson)},
							},
						}, nil
					},
				}
			},
			expectedError: ErrNfdSplitOwnership,
			expectedRRs:   nil,
		},
		{
			name:  "bare label subdomain - grafana.corvid.algo",
			qname: "grafana.corvid.algo.",
			nfdRRHandler: func(handler *nfdRRHandler) {
				handler.nfdFetcher = &mockNfdFetcher{
					fetchFunc: func(ctx context.Context, log clog.P, names []string) (map[string]Properties, error) {
						dummyAccount := crypto.GenerateAccount()
						corvidDns := `[{"name":"@","rrData":["72.60.148.52"],"type":"a","ttl":3600},{"name":"www","rrData":["corvid.algo.xyz"],"type":"cname","ttl":3600},{"name":"@","rrData":["2a02:4780:66:5c13::1"],"type":"aaaa","ttl":3600},{"name":"grafana","rrData":["72.60.148.52"],"type":"a","ttl":3600}]`
						// Only corvid.algo exists - grafana.corvid.algo is NOT a segment NFD
						return map[string]Properties{
							"corvid.algo": {
								Internal:    map[string]string{"name": "corvid.algo", "owner": dummyAccount.Address.String()},
								UserDefined: map[string]string{"dns": corvidDns},
							},
						}, nil
					},
				}
			},
			expectedError: nil,
			expectedRRs: []JsonRr{
				{Name: "corvid.algo.", RrData: []string{"72.60.148.52"}, Type: "a", Ttl: 3600},
				{Name: "www.corvid.algo.", RrData: []string{"corvid.algo.xyz"}, Type: "cname", Ttl: 3600},
				{Name: "corvid.algo.", RrData: []string{"2a02:4780:66:5c13::1"}, Type: "aaaa", Ttl: 3600},
				{Name: "grafana.corvid.algo.", RrData: []string{"72.60.148.52"}, Type: "a", Ttl: 3600},
			},
		},
		{
			name:  "root domain with mixed record types - corvid.algo",
			qname: "corvid.algo.",
			nfdRRHandler: func(handler *nfdRRHandler) {
				handler.nfdFetcher = &mockNfdFetcher{
					fetchFunc: func(ctx context.Context, log clog.P, names []string) (map[string]Properties, error) {
						dummyAccount := crypto.GenerateAccount()
						corvidDns := `[{"name":"@","rrData":["72.60.148.52"],"type":"a","ttl":3600},{"name":"@","rrData":["2a02:4780:66:5c13::1"],"type":"aaaa","ttl":3600},{"name":"@","rrData":["10 mail.protonmail.ch","20 mailsec.protonmail.ch"],"type":"mx","ttl":3600},{"name":"@","rrData":["0 issue \"letsencrypt.org\""],"type":"caa","ttl":3600},{"name":"grafana","rrData":["72.60.148.52"],"type":"a","ttl":3600}]`
						return map[string]Properties{
							"corvid.algo": {
								Internal:    map[string]string{"name": "corvid.algo", "owner": dummyAccount.Address.String()},
								UserDefined: map[string]string{"dns": corvidDns},
							},
						}, nil
					},
				}
			},
			expectedError: nil,
			expectedRRs: []JsonRr{
				{Name: "corvid.algo.", RrData: []string{"72.60.148.52"}, Type: "a", Ttl: 3600},
				{Name: "corvid.algo.", RrData: []string{"2a02:4780:66:5c13::1"}, Type: "aaaa", Ttl: 3600},
				{Name: "corvid.algo.", RrData: []string{"10 mail.protonmail.ch", "20 mailsec.protonmail.ch"}, Type: "mx", Ttl: 3600},
				{Name: "corvid.algo.", RrData: []string{"0 issue \"letsencrypt.org\""}, Type: "caa", Ttl: 3600},
				{Name: "grafana.corvid.algo.", RrData: []string{"72.60.148.52"}, Type: "a", Ttl: 3600},
			},
		},
		{
			name:  "regular segment fetch",
			qname: "defi.nfdomains.algo.",
			nfdRRHandler: func(handler *nfdRRHandler) {
				handler.nfdFetcher = &mockNfdFetcher{
					fetchFunc: func(ctx context.Context, log clog.P, names []string) (map[string]Properties, error) {
						rootDns := []JsonRr{
							{
								Name:   "nfdomains.algo.",
								RrData: []string{},
							},
						}
						rootJson, _ := json.Marshal(rootDns)
						segmentDns := []JsonRr{
							{
								Name:   "defi.nfdomains.algo.",
								RrData: []string{},
							},
						}
						segentJson, _ := json.Marshal(segmentDns)

						return map[string]Properties{
							"nfdomains.algo": {
								Internal:    map[string]string{"name": "nfdomains.algo", "owner": "owner1"},
								UserDefined: map[string]string{"dns": string(rootJson)},
							},
							"defi.nfdomains.algo": {
								Internal:    map[string]string{"name": "defi.nfdomains.algo", "owner": "owner1"},
								UserDefined: map[string]string{"dns": string(segentJson)},
							},
						}, nil
					},
				}
			},
			expectedError: nil,
			// expect merged values
			expectedRRs: []JsonRr{
				{
					Name:   "nfdomains.algo.",
					RrData: []string{},
				},
				{
					Name:   "defi.nfdomains.algo.",
					RrData: []string{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &nfdRRHandler{
				nfdCache: expirable.NewLRU[string, Properties](10, nil, time.Minute),
				rrCache:  expirable.NewLRU[string, []JsonRr](10, nil, time.Minute),
			}
			tt.nfdRRHandler(handler)
			log := clog.NewWithPlugin("test-plugin")
			gotRRs, gotErr := handler.GetNfdRRs(context.Background(), log, tt.qname)
			if tt.expectedErrIs != nil {
				assert.ErrorIs(t, gotErr, tt.expectedErrIs)
			} else {
				assert.Equal(t, tt.expectedError, gotErr)
			}
			assert.Equal(t, tt.expectedRRs, gotRRs)
		})
	}
}

type mockNfdFetcher struct {
	fetchFunc func(ctx context.Context, log clog.P, names []string) (map[string]Properties, error)
}

func (m *mockNfdFetcher) FetchNfdDnsVals(ctx context.Context, names []string) (map[string]Properties, error) {
	return m.fetchFunc(ctx, clog.P{}, names)
}
