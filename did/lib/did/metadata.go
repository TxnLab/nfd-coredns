/*
 * Copyright (c) 2025-2026. TxnLab Inc.
 * All Rights reserved.
 */

package did

import "time"

// ResolutionResult contains the full DID resolution output per W3C DID Resolution spec.
type ResolutionResult struct {
	DIDDocument        *DIDDocument       `json:"didDocument"`
	ResolutionMetadata ResolutionMetadata `json:"didResolutionMetadata"`
	DocumentMetadata   DocumentMetadata   `json:"didDocumentMetadata"`
}

// ResolutionMetadata contains metadata about the resolution process itself.
type ResolutionMetadata struct {
	ContentType string `json:"contentType"`
	Retrieved   string `json:"retrieved,omitempty"`
	Duration    int64  `json:"duration,omitempty"` // milliseconds
	Error       string `json:"error,omitempty"`
}

// DocumentMetadata contains metadata about the DID document.
type DocumentMetadata struct {
	Created     string `json:"created,omitempty"`
	Updated     string `json:"updated,omitempty"`
	Deactivated bool   `json:"deactivated"`
	VersionID   string `json:"versionId,omitempty"`
	NFDAppID    uint64 `json:"nfdAppId,omitempty"`
}

// Content types for DID resolution.
const (
	ContentTypeDIDJSON   = "application/did+json"
	ContentTypeDIDLDJSON = "application/did+ld+json"
)

// Standard DID resolution error codes per W3C spec.
const (
	ErrorNotFound      = "notFound"
	ErrorInvalidDID    = "invalidDid"
	ErrorDeactivated   = "deactivated"
	ErrorInternalError = "internalError"
)

// NewResolutionMetadata creates metadata with the current timestamp.
func NewResolutionMetadata(contentType string) ResolutionMetadata {
	return ResolutionMetadata{
		ContentType: contentType,
		Retrieved:   time.Now().UTC().Format(time.RFC3339),
	}
}

// ErrorResult returns a ResolutionResult containing only an error.
func ErrorResult(errorCode string, contentType string) *ResolutionResult {
	return &ResolutionResult{
		ResolutionMetadata: ResolutionMetadata{
			ContentType: contentType,
			Retrieved:   time.Now().UTC().Format(time.RFC3339),
			Error:       errorCode,
		},
	}
}
