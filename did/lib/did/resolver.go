/*
 * Copyright (c) 2025. TxnLab Inc.
 * All Rights reserved.
 */

package did

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/hashicorp/golang-lru/v2/expirable"

	"github.com/TxnLab/nfd-coredns/internal/nfd"
)

// NfdDIDResolver resolves did:nfd identifiers to DID Documents.
type NfdDIDResolver interface {
	Resolve(ctx context.Context, did string) (*ResolutionResult, error)
}

type nfdDIDResolver struct {
	fetcher  nfd.NfdFetcher
	docCache *expirable.LRU[string, *ResolutionResult]
}

// validNFD matches root and single-segment NFD names: "name.algo" or "segment.name.algo"
var validNFD = regexp.MustCompile(`^([a-z0-9]{1,27}\.){1,2}algo$`)

// NewNfdDIDResolver creates a new DID resolver backed by an Algorand algod client.
func NewNfdDIDResolver(client *algod.Client, registryID uint64, cacheTTL time.Duration) NfdDIDResolver {
	return &nfdDIDResolver{
		fetcher:  nfd.NewNfdFetcher(client, registryID),
		docCache: expirable.NewLRU[string, *ResolutionResult](50000, nil, cacheTTL),
	}
}

// NewNfdDIDResolverWithFetcher creates a resolver with a custom fetcher (useful for testing).
func NewNfdDIDResolverWithFetcher(fetcher nfd.NfdFetcher, cacheTTL time.Duration) NfdDIDResolver {
	return &nfdDIDResolver{
		fetcher:  fetcher,
		docCache: expirable.NewLRU[string, *ResolutionResult](50000, nil, cacheTTL),
	}
}

// Resolve resolves a did:nfd identifier to a DID Document.
func (r *nfdDIDResolver) Resolve(ctx context.Context, didStr string) (*ResolutionResult, error) {
	start := time.Now()
	contentType := ContentTypeDIDJSON

	// Check cache
	if cached, ok := r.docCache.Get(didStr); ok {
		return cached, nil
	}

	// Parse and validate the DID
	nfdName, err := parseDID(didStr)
	if err != nil {
		return ErrorResult(ErrorInvalidDID, contentType), err
	}

	// Fetch NFD properties from blockchain
	props, nfdAppID, err := r.fetcher.FetchNfdDidVals(ctx, nfdName)
	if err != nil {
		if errors.Is(err, nfd.ErrNfdNotFound) {
			return ErrorResult(ErrorNotFound, contentType), err
		}
		return ErrorResult(ErrorInternalError, contentType), err
	}

	// Build the DID document
	result, err := r.buildResolutionResult(didStr, nfdName, nfdAppID, props, contentType, start)
	if err != nil {
		return ErrorResult(ErrorInternalError, contentType), err
	}

	// Cache the result
	r.docCache.Add(didStr, result)

	return result, nil
}

// parseDID validates and extracts the NFD name from a did:nfd string.
// Both root NFDs (e.g., "patrick.algo") and single-segment NFDs (e.g., "mail.patrick.algo") are valid.
func parseDID(didStr string) (string, error) {
	if !strings.HasPrefix(didStr, MethodPrefix) {
		return "", fmt.Errorf("invalid DID method: must start with %s", MethodPrefix)
	}

	nfdName := didStr[len(MethodPrefix):]
	if !validNFD.MatchString(nfdName) {
		return "", fmt.Errorf("invalid NFD name: %q (must match %s)", nfdName, validNFD.String())
	}

	return nfdName, nil
}

func (r *nfdDIDResolver) buildResolutionResult(
	didStr, nfdName string,
	nfdAppID uint64,
	props nfd.Properties,
	contentType string,
	start time.Time,
) (*ResolutionResult, error) {
	didID := MethodPrefix + nfdName

	// Check deactivation conditions
	deactivated := nfd.IsNFdExpired(props) || !nfd.IsNfdOwned(nfdAppID, props) || props.UserDefined["deactivated"] == "true"

	if deactivated {
		return &ResolutionResult{
			DIDDocument: &DIDDocument{
				Context: DefaultContexts(),
				ID:      didID,
			},
			ResolutionMetadata: ResolutionMetadata{
				ContentType: contentType,
				Retrieved:   time.Now().UTC().Format(time.RFC3339),
				Duration:    time.Since(start).Milliseconds(),
			},
			DocumentMetadata: DocumentMetadata{
				Deactivated: true,
				NFDAppID:    nfdAppID,
			},
		}, nil
	}

	doc := &DIDDocument{
		Context: DefaultContexts(),
		ID:      didID,
	}

	// Controller: default to self, override if u.controller is set
	controller := didID
	if c := props.UserDefined["controller"]; c != "" {
		controller = c
	}
	doc.Controller = controller

	// Build verification methods from owner address
	var verificationMethods []VerificationMethod
	var keyAgreements []VerificationMethod

	ownerAddr := props.Internal["owner"]
	if ownerAddr != "" {
		multibase, err := AlgorandAddressToMultibase(ownerAddr)
		if err == nil {
			vm := VerificationMethod{
				ID:                  didID + FragmentOwner,
				Type:                KeyTypeEd25519,
				Controller:          didID,
				PublicKeyMultibase:  multibase,
				BlockchainAccountId: ownerAddr,
			}
			verificationMethods = append(verificationMethods, vm)
			doc.Authentication = append(doc.Authentication, vm.ID)
			doc.AssertionMethod = append(doc.AssertionMethod, vm.ID)

			// Derive X25519 key for keyAgreement
			pubkey, err := AlgorandAddressToEd25519(ownerAddr)
			if err == nil {
				x25519Key, err := Ed25519ToX25519(pubkey)
				if err == nil {
					keyAgreements = append(keyAgreements, VerificationMethod{
						ID:                 didID + "#x25519-owner",
						Type:               KeyTypeX25519,
						Controller:         didID,
						PublicKeyMultibase: X25519ToMultibase(x25519Key),
					})
				}
			}
		}
	}

	// Build verification methods from verified Algorand addresses (v.caAlgo)
	if caAlgo := props.Verified["caAlgo"]; caAlgo != "" {
		addresses := strings.Split(caAlgo, ",")
		for i, addr := range addresses {
			addr = strings.TrimSpace(addr)
			if addr == "" || addr == ownerAddr {
				continue // skip empty or duplicate of owner
			}
			multibase, err := AlgorandAddressToMultibase(addr)
			if err != nil {
				continue
			}
			vm := VerificationMethod{
				ID:                  fmt.Sprintf("%s#algo-%d", didID, i),
				Type:                KeyTypeEd25519,
				Controller:          didID,
				PublicKeyMultibase:  multibase,
				BlockchainAccountId: addr,
			}
			verificationMethods = append(verificationMethods, vm)
		}
	}

	// Parse additional keys from u.keys
	if keysJSON := props.UserDefined["keys"]; keysJSON != "" {
		var additionalKeys []VerificationMethod
		if err := json.Unmarshal([]byte(keysJSON), &additionalKeys); err == nil {
			for i := range additionalKeys {
				// Ensure IDs are properly prefixed
				if !strings.HasPrefix(additionalKeys[i].ID, didID) {
					additionalKeys[i].ID = didID + additionalKeys[i].ID
				}
				if additionalKeys[i].Controller == "" {
					additionalKeys[i].Controller = didID
				}
			}
			verificationMethods = append(verificationMethods, additionalKeys...)
		}
	}

	doc.VerificationMethod = verificationMethods
	doc.KeyAgreement = keyAgreements

	// Build service endpoints from u.service
	if serviceJSON := props.UserDefined["service"]; serviceJSON != "" {
		var services []Service
		if err := json.Unmarshal([]byte(serviceJSON), &services); err == nil {
			for i := range services {
				if !strings.HasPrefix(services[i].ID, didID) {
					services[i].ID = didID + services[i].ID
				}
			}
			doc.Service = services
		}
	}

	// Build alsoKnownAs
	var alsoKnownAs []string
	if bskyDID := props.Verified["blueskydid"]; bskyDID != "" {
		alsoKnownAs = append(alsoKnownAs, bskyDID)
	}
	if akaJSON := props.UserDefined["alsoKnownAs"]; akaJSON != "" {
		var additional []string
		if err := json.Unmarshal([]byte(akaJSON), &additional); err == nil {
			alsoKnownAs = append(alsoKnownAs, additional...)
		}
	}
	if len(alsoKnownAs) > 0 {
		doc.AlsoKnownAs = alsoKnownAs
	}

	// Build metadata
	docMeta := DocumentMetadata{
		Deactivated: false,
		NFDAppID:    nfdAppID,
	}

	resMeta := ResolutionMetadata{
		ContentType: contentType,
		Retrieved:   time.Now().UTC().Format(time.RFC3339),
		Duration:    time.Since(start).Milliseconds(),
	}

	return &ResolutionResult{
		DIDDocument:        doc,
		ResolutionMetadata: resMeta,
		DocumentMetadata:   docMeta,
	}, nil
}

// ParseDID validates and extracts the NFD name from a did:nfd string (exported for testing).
func ParseDID(did string) (string, error) {
	return parseDID(did)
}
