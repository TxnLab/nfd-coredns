# DID:NFD Research Document

Research and background analysis for implementing a W3C Decentralized Identifier (DID) method for Algorand Non-Fungible Domains (NFDs).

## Table of Contents

1. [W3C DID Core 1.0 Summary](#w3c-did-core-10-summary)
2. [Existing Blockchain DID Methods](#existing-blockchain-did-methods)
3. [Algorand Cryptographic Primitives](#algorand-cryptographic-primitives)
4. [NFD Property Model Analysis](#nfd-property-model-analysis)
5. [Design Rationale](#design-rationale)
6. [References](#references)

---

## W3C DID Core 1.0 Summary

The [W3C DID Core 1.0 specification](https://www.w3.org/TR/did-core/) defines a new type of globally unique identifier designed to enable verifiable, decentralized digital identity.

### Core Concepts

- **DID**: A URI that associates a DID subject with a DID document, allowing trustable interactions. Format: `did:<method>:<method-specific-id>`
- **DID Document**: A JSON-LD document containing verification methods (public keys), authentication relationships, service endpoints, and metadata.
- **DID Method**: Defines how a specific DID scheme is created, resolved, updated, and deactivated.
- **DID Resolution**: The process of obtaining a DID document for a given DID.
- **Verification Method**: A set of parameters (typically a public key) used to independently verify a proof.
- **Verification Relationship**: Links a DID subject to a verification method for specific purposes (authentication, assertion, key agreement, etc.).
- **Service Endpoint**: A URI for interacting with the DID subject (messaging, websites, hubs).

### DID Document Structure

Required:
- `id` — The DID itself
- `@context` — JSON-LD context(s)

Optional:
- `controller` — Entity authorized to make changes
- `verificationMethod` — Array of public keys
- `authentication` — Methods for authentication
- `assertionMethod` — Methods for issuing verifiable credentials
- `keyAgreement` — Methods for key agreement (encryption)
- `capabilityInvocation` — Methods for invoking capabilities
- `capabilityDelegation` — Methods for delegating capabilities
- `service` — Array of service endpoints
- `alsoKnownAs` — Alternative identifiers

### Resolution Metadata

DID Resolution returns three objects:
1. **DID Document** — The resolved document
2. **DID Resolution Metadata** — Information about the resolution process (contentType, duration, error)
3. **DID Document Metadata** — Information about the document (created, updated, deactivated, versionId)

### CRUD Operations

Every DID method must define:
- **Create**: How DIDs are created and registered
- **Read (Resolve)**: How DID documents are produced from a DID
- **Update**: How DID documents are modified
- **Deactivate**: How DIDs are rendered inactive

---

## Existing Blockchain DID Methods

### did:ethr (Ethereum)

- **Identifier**: Ethereum address (0x...)
- **Resolution**: Reads ERC-1056 identity registry contract on Ethereum
- **Key type**: secp256k1 (Ethereum native)
- **Update**: Smart contract transactions
- **Pros**: Large ecosystem, well-established
- **Cons**: Gas costs, secp256k1 only (no Ed25519 native support)
- **Spec**: https://github.com/decentralized-identity/ethr-did-resolver

### did:web

- **Identifier**: Domain name (e.g., `did:web:example.com`)
- **Resolution**: HTTPS fetch of `/.well-known/did.json`
- **Key type**: Any
- **Update**: Update the JSON file on the web server
- **Pros**: Simple, works with existing web infrastructure
- **Cons**: Centralized (depends on DNS/HTTPS), no cryptographic proof of control
- **Spec**: https://w3c-ccg.github.io/did-method-web/

### did:ion (Bitcoin/IPFS)

- **Identifier**: Derived from initial recovery key
- **Resolution**: Sidetree protocol on Bitcoin + IPFS
- **Key type**: Any (Ed25519, secp256k1, etc.)
- **Update**: Sidetree operations anchored to Bitcoin
- **Pros**: Highly decentralized, supports key rotation
- **Cons**: Complex infrastructure, slow resolution, high operational cost
- **Spec**: https://identity.foundation/ion/

### did:key

- **Identifier**: Encoded public key (e.g., `did:key:z6Mk...`)
- **Resolution**: Deterministic — DID document derived from the key itself
- **Key type**: Any (multicodec-encoded)
- **Update**: Not possible (static)
- **Pros**: No blockchain needed, instant resolution
- **Cons**: No key rotation, no services, ephemeral identity only
- **Spec**: https://w3c-ccg.github.io/did-method-key/

### did:plc (Bluesky/AT Protocol)

- **Identifier**: Hash of genesis operation
- **Resolution**: PLC directory server
- **Key type**: Any (typically secp256k1, p256)
- **Update**: Signed operations to PLC directory
- **Pros**: Fast, supports rotation, used by AT Protocol/Bluesky
- **Cons**: Semi-centralized (PLC directory), relatively new
- **Relevance**: NFDs already store `v.blueskydid` for Bluesky integration

### did:pkh (Public Key Hash)

- **Identifier**: Blockchain address in CAIP-10 format (e.g., `did:pkh:eip155:1:0x...`)
- **Resolution**: Deterministic — DID document derived from the address itself
- **Key type**: Depends on chain (secp256k1 for Ethereum, Ed25519 for Algorand, etc.)
- **Update**: Not possible (static, like did:key)
- **Pros**: Multi-chain support, simple, no registration needed
- **Cons**: No key rotation, no services, one address = one DID
- **Spec**: https://github.com/w3c-ccg/did-pkh

### did:ens (Ethereum Name Service)

- **Identifier**: ENS name (e.g., `did:ens:vitalik.eth`)
- **Resolution**: Reads ENS registry and resolver contracts on Ethereum
- **Key type**: Any (stored in ENS records)
- **Update**: ENS record updates on Ethereum
- **Reverse resolution**: Supported — address → one primary ENS name (via `addr.reverse` resolver)
- **Pros**: Human-readable, large ecosystem, reverse resolution
- **Cons**: ETH gas costs, reverse returns only one primary name per address, no many-to-many
- **Relevance**: Closest comparable naming system to NFD; important comparison for multi-account features

### Comparison with did:nfd

| Feature | did:ethr | did:web | did:ion | did:key | did:plc | did:pkh | did:ens | **did:nfd** |
|---|---|---|---|---|---|---|---|---|
| Decentralized | Yes | No | Yes | Yes | Partial | Yes | Yes | Yes |
| Human-readable | No | Yes | No | No | No | No | Yes | **Yes** |
| Key rotation | Yes | Yes | Yes | No | Yes | No | Yes | **Yes** |
| Native key type | secp256k1 | Any | Any | Any | Any | Any | Any | **Ed25519** |
| Service endpoints | Yes | Yes | Yes | No | Yes | No | Yes | **Yes** |
| Resolution cost | ETH gas | HTTP | BTC+IPFS | None | HTTP | None | ETH gas | **Algod query** |
| Expiration | No | No | No | No | No | No | Yes | **Yes (native)** |
| Naming | Hash | Domain | Hash | Key | Hash | Address | ENS name | **NFD name** |

**Unique advantages of did:nfd:**
- Human-readable identifiers (`did:nfd:patrick.algo` vs `did:ethr:0x...`)
- Native Ed25519 keys (Algorand uses Ed25519, aligning with modern DID cryptography)
- Built-in expiration mechanism
- Existing ecosystem of 200,000+ registered NFDs
- Low-cost operations on Algorand (~0.001 ALGO per transaction)
- Existing DNS bridge (algo.xyz) provides discoverability
- **Multi-account identity with reverse resolution** (see below)

### Account Binding and Reverse Resolution

A critical differentiator for `did:nfd` is how it handles the relationship between identifiers and accounts. Most DID methods implement a one-to-one binding (one key or address = one DID). NFD implements a verified many-to-many model with full bidirectional resolution.

**Forward resolution** (DID → accounts): Resolving a `did:nfd` identifier returns the owner account plus all verified linked Algorand addresses (`v.caAlgo`). Each linked address is a separate verification method in the DID Document. The linkage is contract-verified — each address must have signed an on-chain transaction authorizing the association.

**Reverse resolution** (account → DIDs): Given any Algorand address, the NFD platform returns all NFDs where that address is the owner or a verified linked address. This answers: *"For this account, what DIDs reference it?"*

**Many-to-many**: A single account can be linked to multiple NFDs, and a single NFD can link to multiple accounts. Both directions are on-chain and publicly verifiable.

| Capability | did:nfd | did:ens | did:ethr | did:ion | did:web | did:key | did:pkh | did:plc | did:btcr |
|------------|---------|---------|----------|---------|---------|---------|---------|---------|----------|
| Multiple accounts per identifier | **Yes** | Partial (multi-chain) | Limited (delegates) | Yes (controllers) | Limited (keys in file) | No | No | Yes (rotation keys) | Limited |
| Reverse lookup (account → identifiers) | **Yes — all linked** | Partial — one primary name | No | No | No | No | No | No | No |
| True many-to-many | **Yes** | No | No | No | No | No | No | No | No |
| On-chain proof of account linkage | **Yes** | Yes | Yes | Yes | No | No | Partial | Partial | Yes |
| Verified (not self-asserted) linkage | **Yes** | N/A | N/A | N/A | No | N/A | N/A | N/A | N/A |

**Key observations:**

1. **did:key and did:pkh** are the most restrictive — one key/address deterministically produces one DID. No reverse discovery, no multi-account, no services.

2. **did:ethr** allows delegates but has no reverse lookup mechanism. You cannot discover all DIDs associated with an Ethereum address.

3. **did:ion** supports multiple controllers via Sidetree operations, but there is no reverse index from a controller key back to all DIDs it controls.

4. **did:ens** is the closest to did:nfd in functionality — it supports reverse resolution and human-readable names. However, ENS reverse resolution returns only **one primary name** per address (set by the user via `setName`), and there is no way to discover all ENS names linked to an address. NFD returns all linked NFDs.

5. **did:nfd is unique** in offering contract-verified, many-to-many account linkage with full bidirectional resolution. The verified addresses in `v.caAlgo` are proven on-chain (the linked account must sign an opt-in transaction), making the linkage as trustworthy as the blockchain consensus itself. No other DID method provides this combination of provable multi-account binding, complete reverse discovery, and human-readable naming.

---

## Algorand Cryptographic Primitives

### Address Format

Algorand addresses encode Ed25519 public keys:

```
Raw Ed25519 public key: 32 bytes
SHA-512/256 checksum:   4 bytes (last 4 bytes of SHA-512/256 of public key)
Total:                  36 bytes
Encoding:               base32 (RFC 4648, no padding)
Result:                 58-character uppercase string
```

### Address Decoding Pipeline

```
Algorand Address (58 chars, base32)
  → base32 decode → 36 bytes
  → first 32 bytes = raw Ed25519 public key
  → last 4 bytes = checksum (SHA-512/256 of public key)
```

### Ed25519 Properties

- **Signature scheme**: EdDSA (Edwards-curve Digital Signature Algorithm)
- **Curve**: Ed25519 (Twisted Edwards curve over GF(2^255 - 19))
- **Key size**: 32-byte public key, 64-byte private key
- **Signature size**: 64 bytes
- **Security level**: ~128 bits

### Ed25519 to X25519 Conversion

For key agreement (encryption), Ed25519 public keys can be converted to X25519 (Curve25519) keys:
- Same underlying curve (birational equivalence)
- Conversion: `x = (1 + y) / (1 - y)` where `y` is the Ed25519 point y-coordinate
- Library: `filippo.io/edwards25519` provides this conversion
- Result: 32-byte X25519 public key for Diffie-Hellman key exchange

### Multibase/Multicodec Encoding

W3C DID specifications use multibase + multicodec for key encoding:

```
Ed25519 public key multicodec prefix: 0xed 0x01
X25519 public key multicodec prefix:  0xec 0x01
Multibase prefix for base58btc:       'z'
```

Pipeline:
```
Raw Ed25519 key (32 bytes)
  → prepend multicodec prefix (0xed, 0x01)
  → base58btc encode
  → prepend 'z' multibase prefix
  → publicKeyMultibase value
```

---

## NFD Property Model Analysis

### Property Namespaces

NFDs store properties in three namespaces:

| Namespace | Prefix | Access | Storage |
|---|---|---|---|
| Internal | `i.` | Read-only (set by contract) | Global state |
| User-defined | `u.` | Owner-writable | Box storage (V2+) |
| Verified | `v.` | Contract-verified | Box storage (V2+) |

### Properties Relevant to DID

**Internal (automatically managed):**

| Property | Type | Description |
|---|---|---|
| `i.name` | string | NFD name (e.g., "patrick.algo") |
| `i.owner` | address | Current owner's Algorand address (Ed25519 public key) |
| `i.expirationTime` | uint64 | Unix timestamp of expiration |
| `i.sellamt` | uint64 | Sale price (0 = not for sale) |
| `i.ver` | string | Contract version (e.g., "3.21") |

**Verified (contract-verified):**

| Property | Type | Description |
|---|---|---|
| `v.caAlgo` | packed addresses | Verified Algorand addresses (comma-separated after decoding) |
| `v.blueskydid` | string | Bluesky DID (e.g., "did:plc:abc123") |

**User-defined (owner-set, for DID):**

| Property | Type | Description |
|---|---|---|
| `u.dns` | JSON | DNS records (existing) |
| `u.service` | JSON | DID service endpoints (new) |
| `u.keys` | JSON | Additional verification methods (new) |
| `u.controller` | string | Override DID controller (new) |
| `u.alsoKnownAs` | JSON | Additional aliases (new) |
| `u.deactivated` | string | Explicit deactivation flag (new) |

### Existing DID Integration

NFDs already have a DID integration precedent:
- `v.blueskydid` stores a Bluesky DID (did:plc:...)
- This is verified by the NFD contract (Bluesky verification flow)
- The DID resolver can include this as an `alsoKnownAs` entry

### Property Size Limits

- Box storage: up to 32,768 bytes per box
- Global state: limited key-value pairs
- JSON values in `u.*` properties are stored as UTF-8 strings
- Split properties (e.g., `bio_00`, `bio_01`) are merged by the fetcher

### NFD Name Validation

Root NFD names match: `^[a-z0-9]{1,27}\.algo$`
- 1-27 lowercase alphanumeric characters before `.algo`
- Segments (e.g., `mail.patrick.algo`) are not separate identities for DID purposes

---

## Design Rationale

### Why NFDs as DIDs?

1. **Identity-first design**: NFDs are already identity records on Algorand, not just names
2. **Cryptographic ownership**: Owner address = Ed25519 public key = verifiable identity
3. **Extensible properties**: `u.*` namespace allows arbitrary DID metadata without contract changes
4. **Existing ecosystem**: 200,000+ NFDs, active community, DNS bridge
5. **Low cost**: Algorand transaction fees (~0.001 ALGO) make updates practical
6. **Built-in lifecycle**: Expiration and ownership transfer map to DID deactivation and key rotation

### Why Not Use Existing Methods?

- **did:web**: Would work for `patrick.algo.xyz` but loses decentralization — depends on algo.xyz server
- **did:key**: No services, no rotation, no human-readable names
- **did:ethr**: Wrong blockchain, wrong key type, no human-readable names
- **did:plc**: Centralized directory, doesn't leverage NFD on-chain data

### Hybrid Storage Model

The design uses a hybrid approach:
- **Derived data**: Owner key, verified addresses, expiration — already on-chain, no extra work
- **Optional data**: Service endpoints, additional keys — stored in `u.*` properties by the NFD owner

This means every NFD automatically has a basic DID document (with at least the owner key) without any additional setup. Advanced features (services, additional keys) are opt-in.

### Resolution Strategy

DID documents are constructed dynamically at resolution time, not stored as blobs:
- Avoids data duplication
- Always reflects current on-chain state
- No migration needed if the DID spec evolves
- Caching at the resolver level provides performance

---

## References

### W3C Specifications
- [DID Core 1.0](https://www.w3.org/TR/did-core/) — W3C Recommendation
- [DID Specification Registries](https://www.w3.org/TR/did-spec-registries/) — Method registry
- [DID Resolution](https://w3c-ccg.github.io/did-resolution/) — Resolution specification
- [Verifiable Credentials Data Model](https://www.w3.org/TR/vc-data-model/) — Related credential spec

### Cryptography
- [Ed25519](https://ed25519.cr.yp.to/) — EdDSA signature scheme
- [RFC 8032](https://tools.ietf.org/html/rfc8032) — Edwards-Curve Digital Signature Algorithm (EdDSA)
- [RFC 7748](https://tools.ietf.org/html/rfc7748) — Elliptic Curves for Security (X25519)
- [Multibase](https://datatracker.ietf.org/doc/html/draft-multiformats-multibase) — Self-describing base encodings
- [Multicodec](https://github.com/multiformats/multicodec) — Compact codec identifier

### DID Methods
- [did:ethr](https://github.com/decentralized-identity/ethr-did-resolver) — Ethereum DID method
- [did:web](https://w3c-ccg.github.io/did-method-web/) — Web DID method
- [did:ion](https://identity.foundation/ion/) — ION (Bitcoin/IPFS) DID method
- [did:key](https://w3c-ccg.github.io/did-method-key/) — Key DID method
- [did:plc](https://web.plc.directory/spec/did-plc) — PLC DID method (Bluesky)

### Algorand
- [Algorand Developer Docs](https://developer.algorand.org/) — Official documentation
- [go-algorand-sdk](https://github.com/algorand/go-algorand-sdk) — Go SDK
- [NFD Documentation](https://docs.nf.domains/) — NFD platform docs

### Key Encoding
- [Ed25519VerificationKey2020](https://w3c-ccg.github.io/lds-ed25519-2020/) — Ed25519 key suite
- [X25519KeyAgreementKey2020](https://w3c-ccg.github.io/lds-x25519-2020/) — X25519 key suite
- [did:key Ed25519](https://w3c-ccg.github.io/did-method-key/#ed25519-x25519) — Ed25519/X25519 in did:key

### Universal Resolver
- [Universal Resolver](https://github.com/decentralized-identity/universal-resolver) — DIF Universal Resolver
- [Universal Resolver Driver Development](https://github.com/decentralized-identity/universal-resolver/blob/main/docs/driver-development.md) — Driver guide
