/*
 * Copyright (c) 2025. TxnLab Inc.
 * All Rights reserved.
 */

package did

// DIDDocument represents a W3C DID Core 1.0 Document.
// See: https://www.w3.org/TR/did-core/
type DIDDocument struct {
	Context            []string             `json:"@context"`
	ID                 string               `json:"id"`
	Controller         string               `json:"controller,omitempty"`
	VerificationMethod []VerificationMethod `json:"verificationMethod,omitempty"`
	Authentication     []string             `json:"authentication,omitempty"`
	AssertionMethod    []string             `json:"assertionMethod,omitempty"`
	KeyAgreement       []VerificationMethod `json:"keyAgreement,omitempty"`
	Service            []Service            `json:"service,omitempty"`
	AlsoKnownAs        []string             `json:"alsoKnownAs,omitempty"`
}

// VerificationMethod represents a cryptographic public key associated with a DID subject.
type VerificationMethod struct {
	ID                  string `json:"id"`
	Type                string `json:"type"`
	Controller          string `json:"controller"`
	PublicKeyMultibase  string `json:"publicKeyMultibase"`
	BlockchainAccountId string `json:"blockchainAccountId,omitempty"`
}

// Service represents a service endpoint associated with a DID subject.
type Service struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	ServiceEndpoint string `json:"serviceEndpoint"`
}

// DID JSON-LD Contexts
const (
	ContextDIDv1   = "https://www.w3.org/ns/did/v1"
	ContextEd25519 = "https://w3id.org/security/suites/ed25519-2020/v1"
	ContextX25519  = "https://w3id.org/security/suites/x25519-2020/v1"
	MethodPrefix   = "did:nfd:"
	KeyTypeEd25519 = "Ed25519VerificationKey2020"
	KeyTypeX25519  = "X25519KeyAgreementKey2020"
	FragmentOwner  = "#owner"
)

// DefaultContexts returns the standard JSON-LD contexts for a did:nfd document.
func DefaultContexts() []string {
	return []string{ContextDIDv1, ContextEd25519, ContextX25519}
}
