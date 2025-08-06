/*
 * Copyright (c) 2025. TxnLab Inc.
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
