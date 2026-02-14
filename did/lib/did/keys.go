/*
 * Copyright (c) 2025. TxnLab Inc.
 * All Rights reserved.
 */

package did

import (
	"crypto/ed25519"
	"encoding/base32"
	"fmt"
	"strings"

	"filippo.io/edwards25519"
	"github.com/mr-tron/base58"
)

// Multicodec prefixes for key types.
var (
	multicodecEd25519 = []byte{0xed, 0x01}
	multicodecX25519  = []byte{0xec, 0x01}
)

// AlgorandAddressToEd25519 decodes an Algorand address (58-char base32) to a raw 32-byte Ed25519 public key.
// Algorand addresses are: base32(pubkey[32] + checksum[4])
func AlgorandAddressToEd25519(address string) (ed25519.PublicKey, error) {
	if len(address) != 58 {
		return nil, fmt.Errorf("invalid Algorand address length: %d (expected 58)", len(address))
	}

	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(address))
	if err != nil {
		return nil, fmt.Errorf("base32 decode error: %w", err)
	}

	if len(decoded) != 36 {
		return nil, fmt.Errorf("decoded address length: %d (expected 36)", len(decoded))
	}

	// First 32 bytes are the raw Ed25519 public key
	pubkey := make([]byte, ed25519.PublicKeySize)
	copy(pubkey, decoded[:32])

	return pubkey, nil
}

// Ed25519ToMultibase encodes a raw Ed25519 public key as a multibase (base58btc, 'z' prefix) string
// with the Ed25519 multicodec prefix (0xed, 0x01).
func Ed25519ToMultibase(pubkey ed25519.PublicKey) string {
	if len(pubkey) != ed25519.PublicKeySize {
		return ""
	}

	// Prepend multicodec prefix
	data := make([]byte, 0, len(multicodecEd25519)+len(pubkey))
	data = append(data, multicodecEd25519...)
	data = append(data, pubkey...)

	// base58btc encode with 'z' multibase prefix
	return "z" + base58.Encode(data)
}

// AlgorandAddressToMultibase converts an Algorand address directly to a multibase-encoded Ed25519 public key.
func AlgorandAddressToMultibase(address string) (string, error) {
	pubkey, err := AlgorandAddressToEd25519(address)
	if err != nil {
		return "", err
	}
	return Ed25519ToMultibase(pubkey), nil
}

// Ed25519ToX25519 converts an Ed25519 public key to an X25519 public key for key agreement.
// This uses the birational equivalence between Ed25519 and Curve25519.
func Ed25519ToX25519(pubkey ed25519.PublicKey) ([]byte, error) {
	if len(pubkey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid Ed25519 public key length: %d", len(pubkey))
	}

	// Parse the Ed25519 point
	point, err := new(edwards25519.Point).SetBytes(pubkey)
	if err != nil {
		return nil, fmt.Errorf("invalid Ed25519 public key: %w", err)
	}

	// Convert to Montgomery form (X25519)
	return point.BytesMontgomery(), nil
}

// X25519ToMultibase encodes a raw X25519 public key as a multibase (base58btc, 'z' prefix) string.
func X25519ToMultibase(pubkey []byte) string {
	if len(pubkey) != 32 {
		return ""
	}

	data := make([]byte, 0, len(multicodecX25519)+len(pubkey))
	data = append(data, multicodecX25519...)
	data = append(data, pubkey...)

	return "z" + base58.Encode(data)
}
