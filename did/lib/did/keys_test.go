/*
 * Copyright (c) 2025. TxnLab Inc.
 * All Rights reserved.
 */

package did

import (
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/base32"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sha512_256 computes SHA-512/256 hash (used by Algorand for address checksums).
func sha512_256sum(data []byte) []byte {
	h := sha512.New512_256()
	h.Write(data)
	return h.Sum(nil)
}

// makeAlgorandAddress creates an Algorand address from an Ed25519 public key for testing.
func makeAlgorandAddress(pub ed25519.PublicKey) string {
	checksumHash := sha512_256sum(pub)
	addrBytes := make([]byte, 36)
	copy(addrBytes[:32], pub)
	copy(addrBytes[32:], checksumHash[28:32])
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(addrBytes)
}

func TestAlgorandAddressToEd25519(t *testing.T) {
	tests := []struct {
		name    string
		address string
		wantErr bool
		keyLen  int
	}{
		{
			name:    "valid address - all zeros",
			address: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ",
			wantErr: false,
			keyLen:  32,
		},
		{
			name:    "invalid length",
			address: "TOOSHORT",
			wantErr: true,
		},
		{
			name:    "invalid base32",
			address: "!!!INVALIDBASE32INVALIDBASE32INVALIDBASE32INVALIDBASE32INVA",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pubkey, err := AlgorandAddressToEd25519(tt.address)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, pubkey, tt.keyLen)
		})
	}
}

func TestAlgorandAddressRoundTrip(t *testing.T) {
	// Generate a known Ed25519 key pair
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	address := makeAlgorandAddress(pub)

	// Now decode it back
	decoded, err := AlgorandAddressToEd25519(address)
	require.NoError(t, err)
	assert.Equal(t, []byte(pub), []byte(decoded))
}

func TestEd25519ToMultibase(t *testing.T) {
	tests := []struct {
		name   string
		pubkey ed25519.PublicKey
		want   string
	}{
		{
			name:   "zero key",
			pubkey: make(ed25519.PublicKey, 32),
			want:   "z",
		},
		{
			name:   "invalid key length",
			pubkey: make(ed25519.PublicKey, 16),
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Ed25519ToMultibase(tt.pubkey)
			if tt.want == "" {
				assert.Empty(t, result)
			} else {
				assert.True(t, strings.HasPrefix(result, "z"), "multibase should start with 'z'")
				assert.Greater(t, len(result), 1, "should have content after prefix")
			}
		})
	}
}

func TestEd25519ToMultibase_Deterministic(t *testing.T) {
	key := make(ed25519.PublicKey, 32)
	for i := range key {
		key[i] = byte(i)
	}

	result1 := Ed25519ToMultibase(key)
	result2 := Ed25519ToMultibase(key)
	assert.Equal(t, result1, result2)
	assert.True(t, strings.HasPrefix(result1, "z"))
}

func TestAlgorandAddressToMultibase(t *testing.T) {
	address := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ"
	multibase, err := AlgorandAddressToMultibase(address)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(multibase, "z"))

	_, err = AlgorandAddressToMultibase("invalid")
	assert.Error(t, err)
}

func TestEd25519ToX25519(t *testing.T) {
	// Invalid key length
	_, err := Ed25519ToX25519(make(ed25519.PublicKey, 16))
	assert.Error(t, err)
}

func TestEd25519ToX25519_ValidKey(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	x25519Key, err := Ed25519ToX25519(pub)
	require.NoError(t, err)
	assert.Len(t, x25519Key, 32)

	// Same key should produce same X25519 key (deterministic)
	x25519Key2, err := Ed25519ToX25519(pub)
	require.NoError(t, err)
	assert.Equal(t, x25519Key, x25519Key2)
}

func TestX25519ToMultibase(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 100)
	}

	result := X25519ToMultibase(key)
	assert.True(t, strings.HasPrefix(result, "z"))
	assert.Greater(t, len(result), 1)

	// Invalid length
	assert.Empty(t, X25519ToMultibase(make([]byte, 16)))
}

func TestFullPipeline(t *testing.T) {
	// Test the full pipeline: generate key -> Algorand address -> decode -> multibase
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	address := makeAlgorandAddress(pub)

	// Pipeline: address -> Ed25519 -> multibase
	multibase, err := AlgorandAddressToMultibase(address)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(multibase, "z"))

	// Also test X25519 derivation from the same key
	decoded, err := AlgorandAddressToEd25519(address)
	require.NoError(t, err)

	x25519Key, err := Ed25519ToX25519(decoded)
	require.NoError(t, err)

	x25519Multibase := X25519ToMultibase(x25519Key)
	assert.True(t, strings.HasPrefix(x25519Multibase, "z"))

	// Ed25519 and X25519 multibase should be different
	assert.NotEqual(t, multibase, x25519Multibase)
}
