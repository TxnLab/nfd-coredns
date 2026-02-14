# Reverse Resolution Research: DID Discovery from Accounts

Research and design for adding reverse resolution (account → DIDs) to the `did:nfd` method. This document captures the standards landscape, NFD's on-chain capabilities, comparisons with other systems, and a proposed design for future implementation.

---

## Table of Contents

1. [Standards Landscape](#1-standards-landscape)
2. [NFD On-Chain Reverse Lookup](#2-nfd-on-chain-reverse-lookup)
3. [Comparison with Other Systems](#3-comparison-with-other-systems)
4. [Proposed Method Spec Addition](#4-proposed-method-spec-addition)
5. [Proposed Implementation Design](#5-proposed-implementation-design)
6. [References](#6-references)

---

## 1. Standards Landscape

### No Standard Exists

As of 2026, **no W3C or DIF standard defines DID reverse resolution** — the ability to discover one or more DIDs from a given key, address, or account. This is a gap in the ecosystem.

### W3C DID Core 1.0

The W3C DID Core 1.0 specification explicitly declares that mapping human-friendly identifiers to DIDs is **out-of-scope**:

> "The problem of mapping human-friendly identifiers to DIDs (and doing so in a way that can be verified and trusted) is out-of-scope for the DID specification, with solutions expected to be defined in separate specifications that reference the DID specification."

DID Core defines forward resolution (DID → DID Document) as the primary operation. It does not mention, suggest, or provide hooks for reverse resolution.

### DID Resolution Specification (v0.3)

The [DID Resolution spec](https://w3c-ccg.github.io/did-resolution/) defines:
- Forward resolution: DID → DID Document
- DID URL dereferencing
- Resolution metadata and algorithms

It does **not** address reverse resolution in any form. The spec's scope is strictly forward: given a DID, produce a document.

### DID Traits (DIF)

The [DID Traits specification](https://identity.foundation/did-traits/) defines machine-readable characteristics for DID methods (update support, deactivation, key types, hosting model, etc.). It does **not** include a trait for reverse resolution or discovery capability.

### DIF Identifiers & Discovery Working Group

The DIF [Identifiers & Discovery WG](https://identity.foundation/working-groups/identifiers-discovery.html) works on:
- Universal Resolver (forward resolution only)
- Universal Registrar (DID creation/updates)
- `.well-known/did-configuration` (domain → DID, not address → DID)
- Peer DID Method
- DID Specification Extensions

There are **no active proposals** for standardizing reverse resolution.

### Individual DID Method Specifications

No major DID method formally specifies reverse resolution:

| Method | Reverse Resolution in Spec? | Notes |
|--------|---------------------------|-------|
| did:key | No | Purely generative; one key = one DID. Trivial reverse (decode DID → key) but no discovery. |
| did:web | No | Domain-based. No mechanism to discover DIDs from keys. |
| did:ion | No | Multiple controllers supported, but no reverse index. |
| did:ethr | No | Address-based DID, but no discovery of delegated relationships. |
| did:pkh | No | Deterministic address → DID, but no multi-account discovery. |
| did:plc | No | PLC directory supports forward resolution only. |
| did:btcr | No | Transaction-reference based; no reverse from key to DID. |
| did:ens | **No** | Despite ENS having ENSIP-3 reverse resolution at the protocol level, the did:ens method spec does not specify or leverage it. |

### ENS Reverse Resolution (ENSIP-3)

ENS is the most relevant precedent. At the protocol level, ENS has mature reverse resolution ([ENSIP-3](https://docs.ens.domains/ensip/3/)):
- Reverse records stored under `addr.reverse` domain
- `setName()` function to set primary name for an address
- `name(bytes32 node)` resolver function to read it

However:
- Returns only **one primary name** per address (not all linked names)
- The primary name must be explicitly set by the address owner
- The did:ens method specification does not reference or formalize this capability
- No many-to-many relationship

### .well-known/did-configuration

The [Well-Known DID Configuration](https://identity.foundation/well-known-did-configuration/) spec enables domain → DID discovery via `.well-known/did-configuration.json`. This is:
- Domain-to-DID only (not account/key-to-DID)
- Requires the domain controller to host a signed credential
- Not applicable to blockchain address reverse resolution

### Conclusion

**did:nfd would be the first DID method to formally specify reverse resolution.** This is novel territory with no precedent in W3C or DIF standards. The capability exists because NFD's on-chain registry maintains address-to-NFD mappings as part of its core protocol — it's not a bolted-on feature but a native capability of the verifiable data registry.

---

## 2. NFD On-Chain Reverse Lookup

### Registry Box Storage

The NFD Registry contract (Application ID `760937186` on mainnet) maintains box storage for both forward and reverse lookups:

**Forward lookup (name → NFD):**
- Box key: `sha256("name/" + nfdName)`
- Box data: `{ASA ID (8 bytes)}{App ID (8 bytes)}` — packed 64-bit big-endian integers
- Used by: `GetRegistryBoxNameForNFD()` in `internal/nfd/fetch.go`
- Existing code: `FindNFDAppIDByName()` queries this box

**Reverse lookup (address → NFDs):**
- Box key: `sha256("addr/algo/" + raw32BytePublicKey)`
- Box data: NFD App ID(s) for all NFDs where this address is linked into that NFD by both the owner and this address.
- The raw public key is the 32-byte Ed25519 key extracted from the Algorand address (not the 58-character base32 encoding)
- **Not yet exposed** in the codebase — `GetRegistryBoxNameForAddress()` needs to be created

### Coverage

The registry maintains reverse mappings for:
- **Owner addresses** (`i.owner`) — when an NFD is owned by an address, the registry box is updated
- **Verified linked addresses** (`v.caAlgo`) — when an address is verified against an NFD, the registry box is updated

This means a single reverse lookup returns **all** NFDs associated with an address, regardless of whether the relationship is ownership or verified linkage. This is the foundation for the many-to-many model described in the method spec.

### Existing Code Patterns

The `getLookupLSIG()` function in `fetch.go` (lines 265-319) already supports both `name/` and `address/` prefixes — see the comment at line 308:

```
// ie: name/patrick.algo, or address/RXZRFW26WYHFV44APFAK4BEMU3P54OBK47LCAZQJPXOTZ4AZPSFDAKLIQY
```

For V2 (box storage), the pattern is:
```go
// Forward lookup
func GetRegistryBoxNameForNFD(nfdName string) []byte {
    hash := sha256.Sum256([]byte("name/" + nfdName))
    return hash[:]
}

// Reverse lookup (proposed)
func GetRegistryBoxNameForAddress(rawPK []byte) []byte {
    hash := sha256.Sum256(append([]byte("addr/algo/"), rawPK...))
    return hash[:]
}
```

For V1 (LSIG-based), the existing infrastructure supports address lookups:
```go
func GetNFDSigAddressLSIG(address string, registryAppID uint64) (crypto.LogicSigAccount, error) {
    return getLookupLSIG("address/", address, registryAppID)
}
```

### Trust Model

Reverse resolution via the registry has the same trust properties as forward resolution:
- The data is stored on the Algorand blockchain
- Box updates require valid transactions against the NFD Registry contract
- The registry contract enforces that reverse mappings are only created/updated for legitimate ownership and verified linkage events
- Any party can independently verify the results by querying the blockchain

---

## 3. Comparison with Other Systems

### Reverse Resolution Capabilities

| System | Forward Resolution | Reverse Resolution | Many-to-Many | On-Chain | Scope |
|--------|-------------------|-------------------|--------------|----------|-------|
| **NFD** | Name → App ID/Properties | Address → all linked NFDs | **Yes** | Yes (algod) | Owner + verified addresses |
| **ENS** | Name → Address/Records | Address → one primary name | No | Yes (Ethereum) | Primary name only |
| **Unstoppable Domains** | Name → Records | Partial (API only) | No | Partial | Owner only |
| **DID Core** | DID → DID Document | Not defined | N/A | N/A | N/A |
| **did:ethr** | DID → DID Document | Not specified | No | N/A | N/A |
| **DNS** | Name → Records | PTR records (IP → name) | Limited | N/A | IP addresses only |

### Key Differentiators

1. **Complete bidirectionality.** NFD reverse resolution returns ALL NFDs for an address, not just a primary name. This enables true discovery.

2. **On-chain verifiability.** Both directions are backed by the same blockchain consensus. No trusted intermediary.

3. **Covers verified linkages.** Unlike ENS (which only reverse-resolves the owner/primary name), NFD's reverse lookup covers both ownership and verified account linkage (`v.caAlgo`), yielding the full identity graph.

4. **Native to the protocol.** The registry maintains reverse mappings as a core feature, not as an optional add-on.

---

## 4. Proposed Method Spec Addition

### New Section 8.5: Reverse Resolution

This section would be added after Section 8.4 (Deactivate) in `DID_NFD_METHOD_SPEC.md`.

```markdown
### 8.5 Reverse Resolution

| Aspect | Details |
|--------|---------|
| **Method** | Query the NFD Registry to discover all NFDs associated with a given Algorand address, including both owned and verified-linked NFDs. |
| **Process** | 1. Validate the Algorand address format. 2. Decode the address to its raw 32-byte Ed25519 public key. 3. Query the NFD Registry box `sha256("addr/algo/" + rawPK)`. 4. Extract the NFD App ID(s) from the box data. 5. For each NFD, read `i.name` from the application's global state and construct `did:nfd:<name>.algo`. |
| **Input** | A valid 58-character Algorand address. |
| **Output** | A `ReverseResolutionResult` containing the queried address, an array of associated `did:nfd:*` identifiers, and resolution metadata. |
| **On-chain** | Fully on-chain via algod. The NFD Registry maintains `addr/algo/{rawPK}` boxes for all addresses that are NFD owners or verified linked accounts (`v.caAlgo`). No external indexer or API is required. |
| **HTTP endpoint** | `GET /1.0/reverse/{address}` on the DID resolver HTTP service. |
| **Caching** | The resolver implementation uses a separate LRU cache for reverse results with the same configurable TTL as forward resolution. |
| **Error codes** | `invalidAddress` (malformed Algorand address), `internalError` (blockchain query failure). |
| **Empty result** | If the address is not associated with any NFD, the result contains an empty `dids` array. This is not an error — it indicates the address has no NFD identity linkage. |

> **Note:** Reverse resolution is a method-specific extension of the `did:nfd` method. It
> is not defined by the W3C DID Core or DID Resolution specifications (which cover
> forward resolution only). This capability is enabled by the NFD Registry's on-chain
> address-to-NFD index. The mapping is stored in registry box storage with key
> `sha256("addr/algo/" + rawPublicKey)`, making reverse resolution as trustworthy
> and decentralized as forward resolution.
```

### Response Format

```json
{
  "address": "RXZRFW26WYHFV44APFAK4BEMU3P54OBK47LCAZQJPXOTZ4AZPSFDAKLIQY",
  "dids": [
    "did:nfd:nfdomains.algo",
    "did:nfd:mail.nfdomains.algo"
  ],
  "reverseResolutionMetadata": {
    "retrieved": "2026-02-14T12:00:00Z",
    "duration": 85
  }
}
```

### HTTP Status Codes

| Code | Meaning | When |
|------|---------|------|
| 200 | OK | Address resolved (even if `dids` is empty) |
| 400 | Bad Request | Invalid Algorand address format |
| 500 | Internal Server Error | Blockchain query failed |

### Table of Contents Update

Add to the method spec ToC:
```
   - 8.5 [Reverse Resolution](#85-reverse-resolution)
```

---

## 5. Proposed Implementation Design

### Overview

The implementation adds reverse resolution to three layers:
1. **Fetcher** (`internal/nfd/fetch.go`) — on-chain registry query
2. **Resolver** (`did/lib/did/resolver.go`) — DID construction + caching
3. **HTTP handler** (`did/cmd/did-resolver/main.go`) — API endpoint

### 5.1 Fetcher Layer

**Interface change** — add to `NfdFetcher`:
```go
type NfdFetcher interface {
    FetchNfdDnsVals(ctx context.Context, names []string) (map[string]Properties, error)
    FetchNfdDidVals(ctx context.Context, name string) (Properties, uint64, error)
    FindNFDsByAddress(ctx context.Context, address string) ([]string, error)  // NEW
}
```

**New helper function:**
```go
// GetRegistryBoxNameForAddress returns the registry box key for an Algorand address reverse lookup.
// The box key is sha256("addr/algo/" + raw32BytePublicKey).
func GetRegistryBoxNameForAddress(address string) ([]byte, error) {
    // Decode Algorand address to raw 32-byte Ed25519 public key
    decoded, err := types.DecodeAddress(address)
    if err != nil {
        return nil, fmt.Errorf("invalid Algorand address: %w", err)
    }
    data := append([]byte("addr/algo/"), decoded[:]...)
    hash := sha256.Sum256(data)
    return hash[:], nil
}
```

**Implementation of `FindNFDsByAddress`:**
```go
func (n *nfdFetcher) FindNFDsByAddress(ctx context.Context, address string) ([]string, error) {
    boxName, err := GetRegistryBoxNameForAddress(address)
    if err != nil {
        return nil, err
    }

    boxValue, err := n.Client.GetApplicationBoxByName(n.RegistryId, boxName).Do(ctx)
    if err != nil {
        // Box not found means no NFDs linked to this address
        if strings.Contains(err.Error(), "404") {
            return nil, nil
        }
        return nil, fmt.Errorf("failed to query registry for address %s: %w", address, err)
    }

    // Parse box data to extract NFD App IDs
    // Format: packed 64-bit big-endian integers (8 bytes each)
    // Each pair is {ASA ID}{App ID} (16 bytes per NFD entry)
    var nfdNames []string
    for offset := 0; offset+16 <= len(boxValue.Value); offset += 16 {
        // Skip ASA ID (first 8 bytes), read App ID (next 8 bytes)
        appID := binary.BigEndian.Uint64(boxValue.Value[offset+8 : offset+16])
        if appID == 0 {
            continue
        }

        // Fetch i.name from the NFD application's global state
        appData, err := n.Client.GetApplicationByID(appID).Do(ctx)
        if err != nil {
            continue // skip NFDs that can't be read
        }
        for _, kv := range appData.Params.GlobalState {
            decodedKey, _ := base64.StdEncoding.DecodeString(kv.Key)
            if string(decodedKey) == "i.name" {
                decodedValue, _ := base64.StdEncoding.DecodeString(kv.Value.Bytes)
                nfdNames = append(nfdNames, string(decodedValue))
                break
            }
        }
    }

    return nfdNames, nil
}
```

> **Note:** The exact box data format for `addr/algo/` boxes needs verification against the
> live registry. The implementation above assumes the same `{ASA ID}{App ID}` packed format
> as the `name/` boxes, with multiple entries for addresses linked to multiple NFDs.

### 5.2 Resolver Layer

**Interface change** — add to `NfdDIDResolver`:
```go
type NfdDIDResolver interface {
    Resolve(ctx context.Context, did string) (*ResolutionResult, error)
    ReverseResolve(ctx context.Context, address string) (*ReverseResolutionResult, error)  // NEW
}
```

**New types** in `did/lib/did/metadata.go`:
```go
// ReverseResolutionResult contains the result of a reverse resolution operation.
type ReverseResolutionResult struct {
    Address  string                    `json:"address"`
    DIDs     []string                  `json:"dids"`
    Metadata ReverseResolutionMetadata `json:"reverseResolutionMetadata"`
}

// ReverseResolutionMetadata contains metadata about the reverse resolution process.
type ReverseResolutionMetadata struct {
    Retrieved string `json:"retrieved,omitempty"`
    Duration  int64  `json:"duration,omitempty"`
}
```

**Implementation:**
```go
func (r *nfdDIDResolver) ReverseResolve(ctx context.Context, address string) (*ReverseResolutionResult, error) {
    start := time.Now()

    // Check cache
    if cached, ok := r.reverseCache.Get(address); ok {
        return cached, nil
    }

    // Validate address format
    if _, err := types.DecodeAddress(address); err != nil {
        return nil, fmt.Errorf("invalidAddress: %s", address)
    }

    // Query fetcher
    nfdNames, err := r.fetcher.FindNFDsByAddress(ctx, address)
    if err != nil {
        return nil, err
    }

    // Build DID identifiers
    var dids []string
    for _, name := range nfdNames {
        dids = append(dids, MethodPrefix+name)
    }

    result := &ReverseResolutionResult{
        Address: address,
        DIDs:    dids,
        Metadata: ReverseResolutionMetadata{
            Retrieved: time.Now().UTC().Format(time.RFC3339),
            Duration:  time.Since(start).Milliseconds(),
        },
    }

    // Cache
    r.reverseCache.Add(address, result)

    return result, nil
}
```

The `nfdDIDResolver` struct adds a second cache:
```go
type nfdDIDResolver struct {
    fetcher      nfd.NfdFetcher
    docCache     *expirable.LRU[string, *ResolutionResult]
    reverseCache *expirable.LRU[string, *ReverseResolutionResult]
}
```

### 5.3 HTTP Handler

**New route** in `main.go`:
```go
mux.HandleFunc("GET /1.0/reverse/{address}", handleReverse(resolver))
```

**Handler:**
```go
func handleReverse(resolver did.NfdDIDResolver) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        address := r.PathValue("address")
        if address == "" {
            writeError(w, http.StatusBadRequest, "missing address parameter")
            return
        }

        result, err := resolver.ReverseResolve(r.Context(), address)
        if err != nil {
            if strings.Contains(err.Error(), "invalidAddress") {
                writeError(w, http.StatusBadRequest, err.Error())
                return
            }
            writeError(w, http.StatusInternalServerError, err.Error())
            return
        }

        w.Header().Set("Content-Type", "application/json")
        enc := json.NewEncoder(w)
        enc.SetIndent("", "  ")
        enc.Encode(result)
    }
}
```

**Update properties endpoint:**
```go
"supportsReverseResolution": true,
```

### 5.4 Tests

**Mock fetcher update** — add to `mockNfdFetcher` in resolver tests:
```go
func (m *mockNfdFetcher) FindNFDsByAddress(ctx context.Context, address string) ([]string, error) {
    if names, ok := m.addressMap[address]; ok {
        return names, nil
    }
    return nil, nil
}
```

**Test cases:**
- Valid address with multiple NFD results
- Valid address with single NFD result
- Valid address with no results (empty `dids` array, not error)
- Invalid address format (400 error)
- Fetcher error (500 error)

### 5.5 User Guide Updates

Add to API Reference section in `DID_NFD_USER_GUIDE.md`:

```markdown
### GET /1.0/reverse/{address}

Reverse resolve an Algorand address to discover all associated DIDs.

**Request:**
```
GET /1.0/reverse/RXZRFW26WYHFV44APFAK4BEMU3P54OBK47LCAZQJPXOTZ4AZPSFDAKLIQY
```

**Response (200 OK):**
```json
{
  "address": "RXZRFW26WYHFV44APFAK4BEMU3P54OBK47LCAZQJPXOTZ4AZPSFDAKLIQY",
  "dids": [
    "did:nfd:nfdomains.algo",
    "did:nfd:mail.nfdomains.algo"
  ],
  "reverseResolutionMetadata": {
    "retrieved": "2026-02-14T12:00:00Z",
    "duration": 85
  }
}
```

**Response (200 OK) -- no associated NFDs:**
```json
{
  "address": "AAAA...YYYY",
  "dids": [],
  "reverseResolutionMetadata": {
    "retrieved": "2026-02-14T12:00:00Z",
    "duration": 42
  }
}
```
```

Add to Quick Reference table:
```
| Find DIDs for an address | `curl http://localhost:8080/1.0/reverse/{address}` |
```

---

## 6. References

### W3C / DIF Specifications
- [W3C DID Core 1.0](https://www.w3.org/TR/did-core/) — Out-of-scope statement on reverse resolution
- [DID Resolution v0.3](https://w3c-ccg.github.io/did-resolution/) — Forward resolution only
- [DID Traits](https://identity.foundation/did-traits/) — No reverse resolution trait
- [Well-Known DID Configuration](https://identity.foundation/well-known-did-configuration/) — Domain→DID only
- [DIF Identifiers & Discovery WG](https://identity.foundation/working-groups/identifiers-discovery.html)

### ENS Reverse Resolution
- [ENSIP-3: Reverse Resolution](https://docs.ens.domains/ensip/3/) — ENS protocol-level reverse resolution
- [ERC-181: ENS Reverse Resolution](https://eips.ethereum.org/EIPS/eip-181) — Ethereum standard
- [did:ens Specification](https://github.com/veramolabs/did-ens-spec) — Does not leverage ENSIP-3

### DID Method Specifications
- [did:key](https://w3c-ccg.github.io/did-method-key/) — No reverse resolution
- [did:web](https://w3c-ccg.github.io/did-method-web/) — No reverse resolution
- [did:ion](https://identity.foundation/ion/) — No reverse resolution
- [did:ethr](https://github.com/decentralized-identity/ethr-did-resolver) — No reverse resolution
- [did:pkh](https://github.com/w3c-ccg/did-pkh) — No reverse resolution
- [did:plc](https://web.plc.directory/spec/did-plc) — No reverse resolution
- [did:btcr](https://w3c-ccg.github.io/didm-btcr/) — No reverse resolution

### NFD
- [NFD Documentation](https://docs.nf.domains/) — NFD platform documentation
- [NFD API - Address Resolution](https://api-docs.nf.domains/reference/integrators-guide/resolving-an-algorand-address-to-an-nfd-name-avatar) — Platform API reverse lookup
- NFD Registry Contract: Application ID `760937186` (mainnet)
