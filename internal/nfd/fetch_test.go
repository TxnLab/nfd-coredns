/*
 * Copyright (c) 2025-2026. TxnLab Inc.
 * All Rights reserved.
 */

package nfd

import (
	"strconv"
	"testing"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/crypto"
	"github.com/stretchr/testify/assert"
)

func TestIsContractVersionAtLeast(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		major    int
		minor    int
		expected bool
	}{
		{
			name:     "3.0 >= 2.1",
			version:  "3.0",
			major:    2,
			minor:    1,
			expected: true,
		},
		{
			name:     "2.1 >= 2.1",
			version:  "2.1",
			major:    2,
			minor:    1,
			expected: true,
		},
		{
			name:     "2.0 < 2.1",
			version:  "2.0",
			major:    2,
			minor:    1,
			expected: false,
		},
		{
			name:     "1.9 < 2.1",
			version:  "1.9",
			major:    2,
			minor:    1,
			expected: false,
		},
		{
			name:     "empty version",
			version:  "",
			major:    3,
			minor:    0,
			expected: false,
		},
		{
			name:     "3.5 >= 3.0",
			version:  "3.5",
			major:    3,
			minor:    0,
			expected: true,
		},
		{
			name:     "3.0 >= 3.0",
			version:  "3.0",
			major:    3,
			minor:    0,
			expected: true,
		},
		{
			name:     "2.9 < 3.0",
			version:  "2.9",
			major:    3,
			minor:    0,
			expected: false,
		},
		{
			name:     "invalid version string",
			version:  "abc",
			major:    1,
			minor:    0,
			expected: false,
		},
		{
			name:     "version with patch",
			version:  "3.1.2",
			major:    3,
			minor:    0,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsContractVersionAtLeast(tt.version, tt.major, tt.minor)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsNFdExpired(t *testing.T) {
	tests := []struct {
		name     string
		props    Properties
		expected bool
	}{
		{
			name: "no expiration time",
			props: Properties{
				Internal: map[string]string{},
			},
			expected: false,
		},
		{
			name: "zero expiration time",
			props: Properties{
				Internal: map[string]string{"expirationTime": "0"},
			},
			expected: false,
		},
		{
			name: "future expiration",
			props: Properties{
				Internal: map[string]string{
					"expirationTime": formatTimestamp(time.Now().Add(time.Hour).Unix()),
				},
			},
			expected: false,
		},
		{
			name: "past expiration",
			props: Properties{
				Internal: map[string]string{
					"expirationTime": formatTimestamp(time.Now().Add(-time.Hour).Unix()),
				},
			},
			expected: true,
		},
		{
			name: "invalid expiration time",
			props: Properties{
				Internal: map[string]string{"expirationTime": "invalid"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNFdExpired(tt.props)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsNfdOwned(t *testing.T) {
	// Generate a test account for NFD app address calculation
	nfdAppId := uint64(12345678)
	nfdAppAddress := crypto.GetApplicationAddress(nfdAppId).String()

	tests := []struct {
		name     string
		nfdAppId uint64
		props    Properties
		expected bool
	}{
		{
			name:     "owned - no sell amount, different owner",
			nfdAppId: nfdAppId,
			props: Properties{
				Internal: map[string]string{
					"sellamt": "0",
					"owner":   "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ",
				},
			},
			expected: true,
		},
		{
			name:     "not owned - has sell amount",
			nfdAppId: nfdAppId,
			props: Properties{
				Internal: map[string]string{
					"sellamt": "1000000",
					"owner":   "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ",
				},
			},
			expected: false,
		},
		{
			name:     "not owned - owner is nfd app address",
			nfdAppId: nfdAppId,
			props: Properties{
				Internal: map[string]string{
					"sellamt": "0",
					"owner":   nfdAppAddress,
				},
			},
			expected: false,
		},
		{
			name:     "owned - empty sellamt treated as 0",
			nfdAppId: nfdAppId,
			props: Properties{
				Internal: map[string]string{
					"owner": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ",
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNfdOwned(tt.nfdAppId, tt.props)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMergeNFDProperties(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]string
	}{
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: map[string]string{},
		},
		{
			name: "no split keys",
			input: map[string]string{
				"bio":   "Hello",
				"email": "test@example.com",
			},
			expected: map[string]string{
				"bio":   "Hello",
				"email": "test@example.com",
			},
		},
		{
			name: "split keys merged",
			input: map[string]string{
				"bio_00": "Hello ",
				"bio_01": "World",
			},
			expected: map[string]string{
				"bio": "Hello World",
			},
		},
		{
			name: "multiple split key groups",
			input: map[string]string{
				"bio_00":     "Bio ",
				"bio_01":     "text",
				"address_00": "123 ",
				"address_01": "Main St",
			},
			expected: map[string]string{
				"bio":     "Bio text",
				"address": "123 Main St",
			},
		},
		{
			name: "mixed split and non-split",
			input: map[string]string{
				"bio_00": "Hello ",
				"bio_01": "World",
				"email":  "test@example.com",
			},
			expected: map[string]string{
				"bio":   "Hello World",
				"email": "test@example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeNFDProperties(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFetchAlgoAddressesFromPackedValue(t *testing.T) {
	// Create a valid 32-byte Algorand address
	testAccount := crypto.GenerateAccount()
	testAddr := testAccount.Address

	// Create a zero address (32 zero bytes)
	var zeroAddr [32]byte

	tests := []struct {
		name        string
		data        []byte
		expected    []string
		expectError bool
	}{
		{
			name:        "empty data",
			data:        []byte{},
			expected:    nil,
			expectError: false,
		},
		{
			name:        "invalid length (not multiple of 32)",
			data:        make([]byte, 33),
			expected:    nil,
			expectError: true,
		},
		{
			name:        "single valid address",
			data:        testAddr[:],
			expected:    []string{testAddr.String()},
			expectError: false,
		},
		{
			name:        "single zero address (skipped)",
			data:        zeroAddr[:],
			expected:    nil,
			expectError: false,
		},
		{
			name:        "multiple addresses with one zero",
			data:        append(testAddr[:], zeroAddr[:]...),
			expected:    []string{testAddr.String()},
			expectError: false,
		},
		{
			name:        "two valid addresses",
			data:        append(testAddr[:], testAddr[:]...),
			expected:    []string{testAddr.String(), testAddr.String()},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FetchAlgoAddressesFromPackedValue(tt.data)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestGetRegistryBoxNameForNFD(t *testing.T) {
	// The function should return a SHA256 hash of "name/" + nfdName
	tests := []struct {
		name    string
		nfdName string
	}{
		{
			name:    "simple name",
			nfdName: "patrick.algo",
		},
		{
			name:    "segment name",
			nfdName: "foo.patrick.algo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetRegistryBoxNameForNFD(tt.nfdName)
			// Should always return 32 bytes (SHA256 hash length)
			assert.Len(t, result, 32)
		})
	}
}

func TestIsNFdExpiredWithRealTimestamps(t *testing.T) {
	futureTime := time.Now().Add(time.Hour).Unix()
	pastTime := time.Now().Add(-time.Hour).Unix()

	tests := []struct {
		name     string
		props    Properties
		expected bool
	}{
		{
			name: "future expiration - not expired",
			props: Properties{
				Internal: map[string]string{
					"expirationTime": formatTimestamp(futureTime),
				},
			},
			expected: false,
		},
		{
			name: "past expiration - expired",
			props: Properties{
				Internal: map[string]string{
					"expirationTime": formatTimestamp(pastTime),
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNFdExpired(tt.props)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func formatTimestamp(t int64) string {
	return strconv.FormatInt(t, 10)
}
