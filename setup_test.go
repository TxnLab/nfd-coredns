/*
 * Copyright (c) 2025. TxnLab Inc.
 * All Rights reserved.
 */

package main

import (
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNfdParse(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectError   bool
		errorContains string
		validate      func(t *testing.T, cfg *nfdPluginConfig)
	}{
		{
			name: "valid minimal config",
			input: `nfd {
				node https://mainnet-api.4160.nodely.dev
			}`,
			expectError: false,
			validate: func(t *testing.T, cfg *nfdPluginConfig) {
				assert.Equal(t, "https://mainnet-api.4160.nodely.dev", cfg.nodeUrl)
				// Defaults
				assert.Equal(t, uint64(defRegId), cfg.registryID)
				assert.Equal(t, defAlgoXyzIp, cfg.algoXyzIp)
				assert.Equal(t, defCacheMinutes, cfg.cacheMins)
			},
		},
		{
			name: "valid full config",
			input: `nfd {
				node https://mainnet-api.4160.nodely.dev
				token mytoken123
				registryid 12345678
				algoxyzip 192.168.1.100
				cachemins 10
			}`,
			expectError: false,
			validate: func(t *testing.T, cfg *nfdPluginConfig) {
				assert.Equal(t, "https://mainnet-api.4160.nodely.dev", cfg.nodeUrl)
				assert.Equal(t, "mytoken123", cfg.token)
				assert.Equal(t, uint64(12345678), cfg.registryID)
				assert.Equal(t, "192.168.1.100", cfg.algoXyzIp)
				assert.Equal(t, 10, cfg.cacheMins)
			},
		},
		{
			name: "missing node",
			input: `nfd {
			}`,
			expectError:   true,
			errorContains: "no node",
		},
		{
			name: "node with no value",
			input: `nfd {
				node
			}`,
			expectError:   true,
			errorContains: "invalid node; no value",
		},
		{
			name: "node with multiple values",
			input: `nfd {
				node https://example1.com https://example2.com
			}`,
			expectError:   true,
			errorContains: "invalid node; multiple values",
		},
		{
			name: "token with no value",
			input: `nfd {
				node https://mainnet-api.4160.nodely.dev
				token
			}`,
			expectError:   true,
			errorContains: "invalid token; no value",
		},
		{
			name: "token with multiple values",
			input: `nfd {
				node https://mainnet-api.4160.nodely.dev
				token tok1 tok2
			}`,
			expectError:   true,
			errorContains: "invalid token; multiple values",
		},
		{
			name: "invalid algoxyzip - not IPv4",
			input: `nfd {
				node https://mainnet-api.4160.nodely.dev
				algoxyzip 2001:db8::1
			}`,
			expectError:   true,
			errorContains: "not a valid IPv4 address",
		},
		{
			name: "invalid algoxyzip - not an IP",
			input: `nfd {
				node https://mainnet-api.4160.nodely.dev
				algoxyzip notanip
			}`,
			expectError:   true,
			errorContains: "not a valid IPv4 address",
		},
		{
			name: "algoxyzip with no value",
			input: `nfd {
				node https://mainnet-api.4160.nodely.dev
				algoxyzip
			}`,
			expectError:   true,
			errorContains: "invalid algoxyzip; no value",
		},
		{
			name: "invalid registryid - not a number",
			input: `nfd {
				node https://mainnet-api.4160.nodely.dev
				registryid notanumber
			}`,
			expectError:   true,
			errorContains: "invalid integer value for registry id",
		},
		{
			name: "registryid with no value",
			input: `nfd {
				node https://mainnet-api.4160.nodely.dev
				registryid
			}`,
			expectError:   true,
			errorContains: "invalid registryid; no value",
		},
		{
			name: "invalid cachemins - not a number",
			input: `nfd {
				node https://mainnet-api.4160.nodely.dev
				cachemins notanumber
			}`,
			expectError:   true,
			errorContains: "invalid integer value for cache minutes",
		},
		{
			name: "cachemins with no value",
			input: `nfd {
				node https://mainnet-api.4160.nodely.dev
				cachemins
			}`,
			expectError:   true,
			errorContains: "invalid cachemins; no value",
		},
		{
			name: "unknown directive",
			input: `nfd {
				node https://mainnet-api.4160.nodely.dev
				unknowndirective somevalue
			}`,
			expectError:   true,
			errorContains: "unknown value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := caddy.NewTestController("dns", tt.input)
			cfg, err := nfdParse(c)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.True(t, strings.Contains(err.Error(), tt.errorContains),
						"expected error containing %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg)
			if tt.validate != nil {
				tt.validate(t, cfg)
			}
		})
	}
}

func TestDefaultValues(t *testing.T) {
	// Verify default constants
	assert.Equal(t, uint64(760937186), uint64(defRegId))
	assert.Equal(t, "34.8.101.7", defAlgoXyzIp)
	assert.Equal(t, 5, defCacheMinutes)
}
