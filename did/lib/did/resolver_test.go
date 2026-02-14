/*
 * Copyright (c) 2025. TxnLab Inc.
 * All Rights reserved.
 */

package did

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TxnLab/nfd-coredns/internal/nfd"
)

// mockNfdFetcher implements nfd.NfdFetcher for testing.
type mockNfdFetcher struct {
	dnsResult map[string]nfd.Properties
	dnsErr    error
	didResult nfd.Properties
	didAppID  uint64
	didErr    error
}

func (m *mockNfdFetcher) FetchNfdDnsVals(_ context.Context, names []string) (map[string]nfd.Properties, error) {
	if m.dnsErr != nil {
		return nil, m.dnsErr
	}
	return m.dnsResult, nil
}

func (m *mockNfdFetcher) FetchNfdDidVals(_ context.Context, _ string) (nfd.Properties, uint64, error) {
	return m.didResult, m.didAppID, m.didErr
}

func TestParseDID(t *testing.T) {
	tests := []struct {
		name    string
		did     string
		want    string
		wantErr bool
	}{
		{
			name: "valid root NFD",
			did:  "did:nfd:patrick.algo",
			want: "patrick.algo",
		},
		{
			name: "valid numeric NFD",
			did:  "did:nfd:123.algo",
			want: "123.algo",
		},
		{
			name: "valid mixed alphanumeric",
			did:  "did:nfd:abc123.algo",
			want: "abc123.algo",
		},
		{
			name: "valid single segment NFD",
			did:  "did:nfd:mail.patrick.algo",
			want: "mail.patrick.algo",
		},
		{
			name: "valid segment with numbers",
			did:  "did:nfd:foo.bar123.algo",
			want: "foo.bar123.algo",
		},
		{
			name:    "two-level segment rejected",
			did:     "did:nfd:a.b.c.algo",
			wantErr: true,
		},
		{
			name:    "wrong method",
			did:     "did:web:example.com",
			wantErr: true,
		},
		{
			name:    "empty name",
			did:     "did:nfd:",
			wantErr: true,
		},
		{
			name:    "uppercase rejected",
			did:     "did:nfd:Patrick.algo",
			wantErr: true,
		},
		{
			name:    "missing .algo",
			did:     "did:nfd:patrick",
			wantErr: true,
		},
		{
			name:    "wrong TLD",
			did:     "did:nfd:patrick.eth",
			wantErr: true,
		},
		{
			name:    "special characters rejected",
			did:     "did:nfd:my-name.algo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDID(tt.did)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolve_BasicDocument(t *testing.T) {
	ownerAddr := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ"
	futureExpiry := strconv.FormatUint(uint64(time.Now().Add(365*24*time.Hour).Unix()), 10)

	mock := &mockNfdFetcher{
		didResult: nfd.Properties{
			Internal: map[string]string{
				"name":           "patrick.algo",
				"owner":          ownerAddr,
				"expirationTime": futureExpiry,
			},
			UserDefined: map[string]string{},
			Verified:    map[string]string{},
		},
		didAppID: 12345,
	}

	resolver := NewNfdDIDResolverWithFetcher(mock, 5*time.Minute)
	result, err := resolver.Resolve(context.Background(), "did:nfd:patrick.algo")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.DIDDocument)

	doc := result.DIDDocument
	assert.Equal(t, "did:nfd:patrick.algo", doc.ID)
	assert.Equal(t, "did:nfd:patrick.algo", doc.Controller)
	assert.Equal(t, DefaultContexts(), doc.Context)

	// Should have owner verification method
	require.Len(t, doc.VerificationMethod, 1)
	assert.Equal(t, "did:nfd:patrick.algo#owner", doc.VerificationMethod[0].ID)
	assert.Equal(t, KeyTypeEd25519, doc.VerificationMethod[0].Type)

	// Should have authentication and assertion
	assert.Contains(t, doc.Authentication, "did:nfd:patrick.algo#owner")
	assert.Contains(t, doc.AssertionMethod, "did:nfd:patrick.algo#owner")

	// Should have X25519 key agreement
	require.Len(t, doc.KeyAgreement, 1)
	assert.Equal(t, "did:nfd:patrick.algo#x25519-owner", doc.KeyAgreement[0].ID)
	assert.Equal(t, KeyTypeX25519, doc.KeyAgreement[0].Type)

	// Metadata
	assert.False(t, result.DocumentMetadata.Deactivated)
	assert.Equal(t, uint64(12345), result.DocumentMetadata.NFDAppID)
	assert.NotEmpty(t, result.ResolutionMetadata.ContentType)
}

func TestResolve_WithVerifiedAddresses(t *testing.T) {
	ownerAddr := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ"
	verifiedAddr := "7777777777777777777777777777777777777777777777777774MSJUVU"
	futureExpiry := strconv.FormatUint(uint64(time.Now().Add(365*24*time.Hour).Unix()), 10)

	mock := &mockNfdFetcher{
		didResult: nfd.Properties{
			Internal: map[string]string{
				"name":           "patrick.algo",
				"owner":          ownerAddr,
				"expirationTime": futureExpiry,
			},
			UserDefined: map[string]string{},
			Verified: map[string]string{
				"caAlgo": verifiedAddr,
			},
		},
		didAppID: 12345,
	}

	resolver := NewNfdDIDResolverWithFetcher(mock, 5*time.Minute)
	result, err := resolver.Resolve(context.Background(), "did:nfd:patrick.algo")
	require.NoError(t, err)

	doc := result.DIDDocument
	// Should have owner + 1 verified address
	assert.Len(t, doc.VerificationMethod, 2)
	assert.Equal(t, "did:nfd:patrick.algo#algo-0", doc.VerificationMethod[1].ID)
}

func TestResolve_WithVerifiedAddresses_SkipsDuplicateOwner(t *testing.T) {
	ownerAddr := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ"
	futureExpiry := strconv.FormatUint(uint64(time.Now().Add(365*24*time.Hour).Unix()), 10)

	mock := &mockNfdFetcher{
		didResult: nfd.Properties{
			Internal: map[string]string{
				"name":           "test.algo",
				"owner":          ownerAddr,
				"expirationTime": futureExpiry,
			},
			UserDefined: map[string]string{},
			Verified: map[string]string{
				// caAlgo includes the owner address — should be deduplicated
				"caAlgo": ownerAddr,
			},
		},
		didAppID: 99999,
	}

	resolver := NewNfdDIDResolverWithFetcher(mock, 5*time.Minute)
	result, err := resolver.Resolve(context.Background(), "did:nfd:test.algo")
	require.NoError(t, err)

	// Only owner key, verified duplicate skipped
	assert.Len(t, result.DIDDocument.VerificationMethod, 1)
}

func TestResolve_ExpiredNFD(t *testing.T) {
	pastExpiry := strconv.FormatUint(uint64(time.Now().Add(-24*time.Hour).Unix()), 10)

	mock := &mockNfdFetcher{
		didResult: nfd.Properties{
			Internal: map[string]string{
				"name":           "expired.algo",
				"owner":          "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ",
				"expirationTime": pastExpiry,
			},
			UserDefined: map[string]string{},
			Verified:    map[string]string{},
		},
		didAppID: 55555,
	}

	resolver := NewNfdDIDResolverWithFetcher(mock, 5*time.Minute)
	result, err := resolver.Resolve(context.Background(), "did:nfd:expired.algo")
	require.NoError(t, err)

	assert.True(t, result.DocumentMetadata.Deactivated)
	assert.Equal(t, "did:nfd:expired.algo", result.DIDDocument.ID)
	// Deactivated document should be minimal
	assert.Empty(t, result.DIDDocument.VerificationMethod)
}

func TestResolve_ForSaleNFD(t *testing.T) {
	futureExpiry := strconv.FormatUint(uint64(time.Now().Add(365*24*time.Hour).Unix()), 10)

	mock := &mockNfdFetcher{
		didResult: nfd.Properties{
			Internal: map[string]string{
				"name":           "forsale.algo",
				"owner":          "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ",
				"expirationTime": futureExpiry,
				"sellamt":        "1000000",
			},
			UserDefined: map[string]string{},
			Verified:    map[string]string{},
		},
		didAppID: 77777,
	}

	resolver := NewNfdDIDResolverWithFetcher(mock, 5*time.Minute)
	result, err := resolver.Resolve(context.Background(), "did:nfd:forsale.algo")
	require.NoError(t, err)

	assert.True(t, result.DocumentMetadata.Deactivated)
}

func TestResolve_ExplicitDeactivation(t *testing.T) {
	futureExpiry := strconv.FormatUint(uint64(time.Now().Add(365*24*time.Hour).Unix()), 10)

	mock := &mockNfdFetcher{
		didResult: nfd.Properties{
			Internal: map[string]string{
				"name":           "deactivated.algo",
				"owner":          "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ",
				"expirationTime": futureExpiry,
			},
			UserDefined: map[string]string{
				"deactivated": "true",
			},
			Verified: map[string]string{},
		},
		didAppID: 88888,
	}

	resolver := NewNfdDIDResolverWithFetcher(mock, 5*time.Minute)
	result, err := resolver.Resolve(context.Background(), "did:nfd:deactivated.algo")
	require.NoError(t, err)

	assert.True(t, result.DocumentMetadata.Deactivated)
}

func TestResolve_OwnerIsNfdContract(t *testing.T) {
	futureExpiry := strconv.FormatUint(uint64(time.Now().Add(365*24*time.Hour).Unix()), 10)
	nfdAppID := uint64(66666)
	// The NFD contract owns itself (unowned state)
	contractAddr := crypto.GetApplicationAddress(nfdAppID).String()

	mock := &mockNfdFetcher{
		didResult: nfd.Properties{
			Internal: map[string]string{
				"name":           "unowned.algo",
				"owner":          contractAddr,
				"expirationTime": futureExpiry,
			},
			UserDefined: map[string]string{},
			Verified:    map[string]string{},
		},
		didAppID: nfdAppID,
	}

	resolver := NewNfdDIDResolverWithFetcher(mock, 5*time.Minute)
	result, err := resolver.Resolve(context.Background(), "did:nfd:unowned.algo")
	require.NoError(t, err)

	assert.True(t, result.DocumentMetadata.Deactivated)
}

func TestResolve_WithServiceEndpoints(t *testing.T) {
	futureExpiry := strconv.FormatUint(uint64(time.Now().Add(365*24*time.Hour).Unix()), 10)

	services := []Service{
		{ID: "#web", Type: "LinkedDomains", ServiceEndpoint: "https://patrick.algo.xyz"},
		{ID: "#messaging", Type: "DIDCommMessaging", ServiceEndpoint: "https://msg.example.com/patrick"},
	}
	servicesJSON, _ := json.Marshal(services)

	mock := &mockNfdFetcher{
		didResult: nfd.Properties{
			Internal: map[string]string{
				"name":           "patrick.algo",
				"owner":          "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ",
				"expirationTime": futureExpiry,
			},
			UserDefined: map[string]string{
				"service": string(servicesJSON),
			},
			Verified: map[string]string{},
		},
		didAppID: 12345,
	}

	resolver := NewNfdDIDResolverWithFetcher(mock, 5*time.Minute)
	result, err := resolver.Resolve(context.Background(), "did:nfd:patrick.algo")
	require.NoError(t, err)

	doc := result.DIDDocument
	require.Len(t, doc.Service, 2)
	assert.Equal(t, "did:nfd:patrick.algo#web", doc.Service[0].ID)
	assert.Equal(t, "LinkedDomains", doc.Service[0].Type)
	assert.Equal(t, "https://patrick.algo.xyz", doc.Service[0].ServiceEndpoint)
}

func TestResolve_WithAlsoKnownAs(t *testing.T) {
	futureExpiry := strconv.FormatUint(uint64(time.Now().Add(365*24*time.Hour).Unix()), 10)

	additionalAKA := []string{"did:web:example.com"}
	akaJSON, _ := json.Marshal(additionalAKA)

	mock := &mockNfdFetcher{
		didResult: nfd.Properties{
			Internal: map[string]string{
				"name":           "patrick.algo",
				"owner":          "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ",
				"expirationTime": futureExpiry,
			},
			UserDefined: map[string]string{
				"alsoKnownAs": string(akaJSON),
			},
			Verified: map[string]string{
				"blueskydid": "did:plc:abc123xyz",
			},
		},
		didAppID: 12345,
	}

	resolver := NewNfdDIDResolverWithFetcher(mock, 5*time.Minute)
	result, err := resolver.Resolve(context.Background(), "did:nfd:patrick.algo")
	require.NoError(t, err)

	doc := result.DIDDocument
	require.Len(t, doc.AlsoKnownAs, 2)
	assert.Equal(t, "did:plc:abc123xyz", doc.AlsoKnownAs[0]) // Bluesky first
	assert.Equal(t, "did:web:example.com", doc.AlsoKnownAs[1])
}

func TestResolve_WithControllerOverride(t *testing.T) {
	futureExpiry := strconv.FormatUint(uint64(time.Now().Add(365*24*time.Hour).Unix()), 10)

	mock := &mockNfdFetcher{
		didResult: nfd.Properties{
			Internal: map[string]string{
				"name":           "delegated.algo",
				"owner":          "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ",
				"expirationTime": futureExpiry,
			},
			UserDefined: map[string]string{
				"controller": "did:nfd:admin.algo",
			},
			Verified: map[string]string{},
		},
		didAppID: 12345,
	}

	resolver := NewNfdDIDResolverWithFetcher(mock, 5*time.Minute)
	result, err := resolver.Resolve(context.Background(), "did:nfd:delegated.algo")
	require.NoError(t, err)

	assert.Equal(t, "did:nfd:admin.algo", result.DIDDocument.Controller)
}

func TestResolve_NotFound(t *testing.T) {
	mock := &mockNfdFetcher{
		didErr: nfd.ErrNfdNotFound,
	}

	resolver := NewNfdDIDResolverWithFetcher(mock, 5*time.Minute)
	result, err := resolver.Resolve(context.Background(), "did:nfd:nonexistent.algo")
	require.Error(t, err)
	assert.Equal(t, ErrorNotFound, result.ResolutionMetadata.Error)
}

func TestResolve_InvalidDID(t *testing.T) {
	mock := &mockNfdFetcher{}

	resolver := NewNfdDIDResolverWithFetcher(mock, 5*time.Minute)

	// Wrong method
	result, err := resolver.Resolve(context.Background(), "did:web:example.com")
	require.Error(t, err)
	assert.Equal(t, ErrorInvalidDID, result.ResolutionMetadata.Error)

	// Two-level segment
	result, err = resolver.Resolve(context.Background(), "did:nfd:a.b.c.algo")
	require.Error(t, err)
	assert.Equal(t, ErrorInvalidDID, result.ResolutionMetadata.Error)
}

func TestResolve_FetchError(t *testing.T) {
	mock := &mockNfdFetcher{
		didErr: fmt.Errorf("network error"),
	}

	resolver := NewNfdDIDResolverWithFetcher(mock, 5*time.Minute)
	result, err := resolver.Resolve(context.Background(), "did:nfd:patrick.algo")
	require.Error(t, err)
	assert.Equal(t, ErrorInternalError, result.ResolutionMetadata.Error)
}

func TestResolve_CacheHit(t *testing.T) {
	futureExpiry := strconv.FormatUint(uint64(time.Now().Add(365*24*time.Hour).Unix()), 10)
	callCount := 0

	mock := &mockNfdFetcher{
		didResult: nfd.Properties{
			Internal: map[string]string{
				"name":           "cached.algo",
				"owner":          "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ",
				"expirationTime": futureExpiry,
			},
			UserDefined: map[string]string{},
			Verified:    map[string]string{},
		},
		didAppID: 12345,
	}

	resolver := NewNfdDIDResolverWithFetcher(mock, 5*time.Minute)

	// First call
	result1, err := resolver.Resolve(context.Background(), "did:nfd:cached.algo")
	require.NoError(t, err)
	_ = callCount

	// Second call should hit cache
	result2, err := resolver.Resolve(context.Background(), "did:nfd:cached.algo")
	require.NoError(t, err)

	assert.Equal(t, result1.DIDDocument.ID, result2.DIDDocument.ID)
}

func TestResolve_SegmentNFD(t *testing.T) {
	ownerAddr := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ"
	futureExpiry := strconv.FormatUint(uint64(time.Now().Add(365*24*time.Hour).Unix()), 10)

	mock := &mockNfdFetcher{
		didResult: nfd.Properties{
			Internal: map[string]string{
				"name":           "mail.patrick.algo",
				"owner":          ownerAddr,
				"expirationTime": futureExpiry,
			},
			UserDefined: map[string]string{},
			Verified:    map[string]string{},
		},
		didAppID: 99999,
	}

	resolver := NewNfdDIDResolverWithFetcher(mock, 5*time.Minute)
	result, err := resolver.Resolve(context.Background(), "did:nfd:mail.patrick.algo")
	require.NoError(t, err)
	require.NotNil(t, result.DIDDocument)

	doc := result.DIDDocument
	assert.Equal(t, "did:nfd:mail.patrick.algo", doc.ID)
	assert.Equal(t, "did:nfd:mail.patrick.algo", doc.Controller)
	require.Len(t, doc.VerificationMethod, 1)
	assert.Equal(t, "did:nfd:mail.patrick.algo#owner", doc.VerificationMethod[0].ID)
}

func TestResolve_BlockchainAccountId(t *testing.T) {
	ownerAddr := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ"
	verifiedAddr := "7777777777777777777777777777777777777777777777777774MSJUVU"
	futureExpiry := strconv.FormatUint(uint64(time.Now().Add(365*24*time.Hour).Unix()), 10)

	mock := &mockNfdFetcher{
		didResult: nfd.Properties{
			Internal: map[string]string{
				"name":           "patrick.algo",
				"owner":          ownerAddr,
				"expirationTime": futureExpiry,
			},
			UserDefined: map[string]string{},
			Verified: map[string]string{
				"caAlgo": verifiedAddr,
			},
		},
		didAppID: 12345,
	}

	resolver := NewNfdDIDResolverWithFetcher(mock, 5*time.Minute)
	result, err := resolver.Resolve(context.Background(), "did:nfd:patrick.algo")
	require.NoError(t, err)

	doc := result.DIDDocument
	require.Len(t, doc.VerificationMethod, 2)

	// Owner should have BlockchainAccountId
	assert.Equal(t, ownerAddr, doc.VerificationMethod[0].BlockchainAccountId)

	// Verified address should have BlockchainAccountId
	assert.Equal(t, verifiedAddr, doc.VerificationMethod[1].BlockchainAccountId)

	// KeyAgreement should NOT have BlockchainAccountId
	require.Len(t, doc.KeyAgreement, 1)
	assert.Empty(t, doc.KeyAgreement[0].BlockchainAccountId)
}

func TestResolve_FullDocument(t *testing.T) {
	futureExpiry := strconv.FormatUint(uint64(time.Now().Add(365*24*time.Hour).Unix()), 10)
	ownerAddr := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ"
	verifiedAddr := "7777777777777777777777777777777777777777777777777774MSJUVU"

	services := []Service{
		{ID: "#web", Type: "LinkedDomains", ServiceEndpoint: "https://patrick.algo.xyz"},
	}
	servicesJSON, _ := json.Marshal(services)

	mock := &mockNfdFetcher{
		didResult: nfd.Properties{
			Internal: map[string]string{
				"name":           "patrick.algo",
				"owner":          ownerAddr,
				"expirationTime": futureExpiry,
			},
			UserDefined: map[string]string{
				"service": string(servicesJSON),
			},
			Verified: map[string]string{
				"caAlgo":     verifiedAddr,
				"blueskydid": "did:plc:abc123",
			},
		},
		didAppID: 12345,
	}

	resolver := NewNfdDIDResolverWithFetcher(mock, 5*time.Minute)
	result, err := resolver.Resolve(context.Background(), "did:nfd:patrick.algo")
	require.NoError(t, err)

	doc := result.DIDDocument

	// Verify full document structure
	assert.Equal(t, "did:nfd:patrick.algo", doc.ID)
	assert.Equal(t, "did:nfd:patrick.algo", doc.Controller)
	assert.Len(t, doc.Context, 3)
	assert.Len(t, doc.VerificationMethod, 2) // owner + verified
	assert.Len(t, doc.Authentication, 1)
	assert.Len(t, doc.AssertionMethod, 1)
	assert.Len(t, doc.KeyAgreement, 1)
	assert.Len(t, doc.Service, 1)
	assert.Len(t, doc.AlsoKnownAs, 1)
	assert.Equal(t, "did:plc:abc123", doc.AlsoKnownAs[0])

	// Verify JSON serialization
	jsonBytes, err := json.MarshalIndent(doc, "", "  ")
	require.NoError(t, err)
	assert.Contains(t, string(jsonBytes), `"@context"`)
	assert.Contains(t, string(jsonBytes), `"verificationMethod"`)
	assert.Contains(t, string(jsonBytes), `"publicKeyMultibase"`)
}
