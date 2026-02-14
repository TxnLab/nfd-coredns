# DID Method Specification: `did:nfd`

**Version:** 1.0

**Authors:** TxnLab Inc.

**Status:** Draft

**Created:** 2025-01-01

**Updated:** 2025-02-13

**Latest version:** https://github.com/TxnLab/nfd-coredns/blob/main/did/docs/DID_NFD_METHOD_SPEC.md

---

## Table of Contents

1. [Introduction](#1-introduction)
2. [Conformance](#2-conformance)
3. [Terminology](#3-terminology)
4. [DID Method Name](#4-did-method-name)
5. [DID Method Specific Identifier](#5-did-method-specific-identifier)
6. [DID URL Syntax](#6-did-url-syntax)
7. [DID Document](#7-did-document)
   - 7.1 [DID Document Construction](#71-did-document-construction)
   - 7.2 [Example DID Document](#72-example-did-document)
8. [CRUD Operations](#8-crud-operations)
   - 8.1 [Create](#81-create)
   - 8.2 [Read (Resolve)](#82-read-resolve)
   - 8.3 [Update](#83-update)
   - 8.4 [Deactivate](#84-deactivate)
9. [NFD Property Mapping](#9-nfd-property-mapping)
   - 9.1 [Derived Properties](#91-derived-properties)
   - 9.2 [Optional User-Defined Properties](#92-optional-user-defined-properties)
   - 9.3 [Multi-Account Identity Model](#93-multi-account-identity-model)
   - 9.4 [Example: Setting Service Endpoints](#94-example-setting-service-endpoints)
10. [Algorand Address to Ed25519 Key Pipeline](#10-algorand-address-to-ed25519-key-pipeline)
    - 10.1 [Pipeline Overview](#101-pipeline-overview)
    - 10.2 [Step-by-Step Conversion](#102-step-by-step-conversion)
    - 10.3 [X25519 Key Derivation](#103-x25519-key-derivation)
11. [Security Considerations](#11-security-considerations)
12. [Privacy Considerations](#12-privacy-considerations)
13. [References](#13-references)

---

## 1. Introduction

The `did:nfd` method is a Decentralized Identifier (DID) method that leverages [Non-Fungible Domains (NFDs)](https://app.nf.domains) on the [Algorand](https://www.algorand.com) blockchain as the verifiable data registry. NFDs are blockchain-based naming identities that map human-readable names (e.g., `nfdomains.algo`) to Algorand accounts and associated metadata. The `did:nfd` method bridges this on-chain naming system to the W3C Decentralized Identifiers ecosystem, enabling NFD owners to participate in decentralized identity protocols without deploying additional infrastructure.

Each NFD is an Algorand Application (smart contract) whose global state and box storage contain the identity properties. The `did:nfd` resolver queries an Algorand algod node to read these on-chain properties and dynamically constructs a conformant DID Document. No off-chain registry, separate anchoring transaction, or dedicated DID contract is required -- the NFD smart contract itself serves as both the naming system and the verifiable data registry.

### Design Goals

- **Leverage existing infrastructure.** NFD owners who have already registered and configured their domains automatically possess a `did:nfd` identifier with no additional on-chain operations.
- **Dynamic resolution.** DID Documents are constructed at resolution time from current blockchain state, ensuring they always reflect the latest on-chain data.
- **Algorand-native cryptography.** Algorand accounts use Ed25519 key pairs. The `did:nfd` method extracts the Ed25519 public key directly from the Algorand address, eliminating the need for separate key registration.
- **Extensibility.** NFD user-defined properties (`u.*`) allow owners to declare service endpoints, additional verification methods, alternative controllers, cross-chain identifiers, and other DID Document elements.
- **Multi-account identity with reverse resolution.** NFDs support provable on-chain linkage of multiple Algorand accounts via verified addresses (`v.caAlgo`). Combined with the NFD platform's reverse lookup capability (any Algorand address → all linked NFDs), `did:nfd` enables a verifiable many-to-many relationship between identifiers and accounts — a feature absent from most DID methods.

---

## 2. Conformance

This DID method specification conforms to the requirements specified in the [W3C Decentralized Identifiers (DIDs) v1.0](https://www.w3.org/TR/did-core/) specification. The `did:nfd` method is designed to produce DID Documents that are valid according to the DID Core data model and can be consumed by any conformant DID resolver or DID-aware application.

The key conformance points are:

- **DID Syntax.** The `did:nfd` identifier conforms to the generic DID syntax defined in [DID Core Section 3.1](https://www.w3.org/TR/did-core/#did-syntax).
- **DID Document.** Resolved DID Documents conform to the DID Core data model, including required properties (`@context`, `id`) and optional properties (`controller`, `verificationMethod`, `authentication`, `assertionMethod`, `keyAgreement`, `service`, `alsoKnownAs`).
- **DID URL.** DID URLs with fragment and query components conform to [DID Core Section 3.2](https://www.w3.org/TR/did-core/#did-url-syntax).
- **DID Resolution.** The resolution process conforms to [DID Resolution v1.0](https://w3c-ccg.github.io/did-resolution/), returning a `ResolutionResult` containing `didDocument`, `didResolutionMetadata`, and `didDocumentMetadata`.
- **Verification Method Types.** Verification methods use `Ed25519VerificationKey2020` and `X25519KeyAgreementKey2020` as defined in the [Ed25519 Signature Suite 2020](https://w3c-ccg.github.io/di-eddsa-2020/) and [X25519 Key Agreement 2020](https://w3id.org/security/suites/x25519-2020) specifications.
- **Key Representation.** Public keys are encoded using `publicKeyMultibase` with base58btc encoding and appropriate multicodec prefixes, conforming to the [Multibase](https://datatracker.ietf.org/doc/html/draft-multiformats-multibase) and [Multicodec](https://github.com/multiformats/multicodec) specifications.

---

## 3. Terminology

| Term | Definition                                                                                                                                                         |
|------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **NFD** | Non-Fungible Domain. A blockchain-based naming identity on Algorand, represented as an Algorand Application (smart contract).                                      |
| **Root NFD** | A top-level NFD name matching the pattern `<name>.algo` (e.g., `nfdomains.algo`).                                                                                    |
| **Segment NFD** | A subdomain of a root NFD (e.g., `mail.nfdomains.algo`). Each segment is an independent NFD with its own Application ID and properties, and constitutes its own DID. |
| **algod** | The Algorand node daemon that serves the blockchain REST API used to query application state.                                                                      |
| **NFD Registry** | The Algorand Application that serves as the central registry for all NFDs. Its Application ID on mainnet is `760937186`.                                           |
| **NFD Application** | The individual Algorand Application (smart contract) representing a single NFD. Each NFD has a unique Application ID.                                              |
| **Global State** | Key-value storage within an Algorand Application's global state, used for internal (`i.*`) properties.                                                             |
| **Box Storage** | Algorand Application box storage, used for user-defined (`u.*`) and verified (`v.*`) properties.                                                                   |
| **v.caAlgo** | Verified Algorand addresses associated with an NFD, stored as packed 32-byte public keys in box storage (`v.caAlgo.0.as` as raw 'set' of PKs).                |

---

## 4. DID Method Name

The method name for this DID method is: **`nfd`**

A DID that uses this method MUST begin with the following prefix:

```
did:nfd:
```

Per the DID Core specification, the method name MUST be lowercase.

---

## 5. DID Method Specific Identifier

The method-specific identifier is an NFD name (root or single-segment). The formal ABNF grammar is:

```abnf
nfd-did        = "did:nfd:" nfd-name
nfd-name       = [ nfd-label "." ] nfd-label "." "algo"
nfd-label      = 1*27( ALPHA / DIGIT )
ALPHA          = %x61-7A           ; a-z (lowercase only)
DIGIT          = %x30-39           ; 0-9
```

As a regular expression:

```
^did:nfd:([a-z0-9]{1,27}\.){1,2}algo$
```

### Constraints

- Each `nfd-label` MUST be between 1 and 27 characters, consisting only of lowercase ASCII letters (`a-z`) and digits (`0-9`).
- Labels MUST NOT contain hyphens, underscores, uppercase characters, or any characters outside the `[a-z0-9]` range.
- The name MUST end with the literal string `.algo`.
- Both root NFDs (`name.algo`) and single-segment NFDs (`segment.name.algo`) are valid DIDs. Each segment NFD is an independent NFD with its own on-chain Application.
- Multi-level segments (e.g., `a.b.c.algo`) MUST be rejected with an `invalidDid` error.

### Examples

| DID | Valid | Notes |
|-----|-------|-------|
| `did:nfd:nfdomains.algo` | Yes | Root NFD |
| `did:nfd:abc123.algo` | Yes | Alphanumeric label |
| `did:nfd:a.algo` | Yes | Minimum label length (1 character) |
| `did:nfd:mail.nfdomains.algo` | Yes | Single-segment NFD |
| `did:nfd:a.b.c.algo` | **No** | Multi-level segment -- not valid |
| `did:nfd:Nfdomains.algo` | **No** | Uppercase characters not permitted |
| `did:nfd:my-name.algo` | **No** | Hyphens not permitted |
| `did:nfd:nfdomains.eth` | **No** | Must end with `.algo` |
| `did:nfd:patrick` | **No** | Missing `.algo` suffix |

---

## 6. DID URL Syntax

DID URLs for the `did:nfd` method follow the generic DID URL syntax defined in [DID Core Section 3.2](https://www.w3.org/TR/did-core/#did-url-syntax):

```
did:nfd:<nfd-name> [ "#" fragment ] [ "?" query ]
```

### Fragments

Fragments identify specific resources within the DID Document, such as verification methods or key agreements.

| DID URL | Resolves To |
|---------|-------------|
| `did:nfd:nfdomains.algo#owner` | The primary verification method derived from the NFD owner's Algorand address |
| `did:nfd:nfdomains.algo#x25519-owner` | The X25519 key agreement key derived from the owner's Ed25519 public key |
| `did:nfd:nfdomains.algo#algo-0` | The first verified Algorand address from `v.caAlgo` (excluding the owner) |
| `did:nfd:nfdomains.algo#key-1` | An additional verification method declared via `u.keys` |
| `did:nfd:nfdomains.algo#web` | The LinkedDomains service endpoint (from `v.domain`, `u.website`, `u.url`, or `u.service`) |
| `did:nfd:nfdomains.algo#profile` | The auto-generated NFDProfile service (from `u.name`, `u.bio`, `u.avatar`, `u.banner`) |
| `did:nfd:nfdomains.algo#twitter` | Auto-generated SocialMedia service for Twitter/X |
| `did:nfd:nfdomains.algo#github` | Auto-generated SocialMedia service for GitHub |
| `did:nfd:nfdomains.algo#bluesky` | Auto-generated SocialMedia service for Bluesky (from `v.blueskydid`) |

### Query Parameters

| Parameter | Description | Example |
|-----------|-------------|---------|
| `service` | Dereference to a named service endpoint URL. Returns the `serviceEndpoint` value of the matching service. | `did:nfd:nfdomains.algo?service=web` |
| `versionTime` | Request the DID Document as it existed at a specific point in time (ISO 8601). Requires historical indexer support. | `did:nfd:nfdomains.algo?versionTime=2025-01-15T00:00:00Z` |

> **Note:** The `versionTime` query parameter depends on the availability of Algorand Indexer services that provide historical application state. When historical state is not available, the resolver SHOULD return the current document with appropriate metadata indicating the limitation.

---

## 7. DID Document

### 7.1 DID Document Construction

The DID Document is constructed dynamically at resolution time by querying the Algorand blockchain. The resolver performs the following steps:

1. **Parse and validate the DID.** Extract the NFD name from the `did:nfd:` prefix. Verify it matches the NFD pattern (`^([a-z0-9]{1,27}\.){1,2}algo$`). Reject multi-level segments and malformed identifiers with an `invalidDid` error.

2. **Look up the NFD Application ID.** Query the NFD Registry contract (Application ID `760937186` on mainnet) to find the Application ID for the given NFD name. The registry stores a mapping from `SHA-256("name/" + nfdName)` to `{ASA ID, App ID}` in box storage. If no mapping exists, fall back to the V1 logic signature lookup. Return `notFound` if the NFD does not exist.

3. **Fetch NFD properties from blockchain.** Query the NFD Application's global state and box storage via the Algorand algod API. The following properties are fetched:
   - **Global state:** Internal properties (`i.owner`, `i.expirationTime`, `i.name`, `i.sellamt`)
   - **Box storage:** Verified properties (`v.domain`, `v.caAlgo.*` packed addresses, `v.blueskydid`, `v.avatar`, `v.banner`, `v.twitter`, `v.discord`, `v.telegram`, `v.github`, `v.linkedin`), user-defined properties (`u.website`, `u.url`, `u.service`, `u.keys`, `u.controller`, `u.alsoKnownAs`, `u.deactivated`, `u.name`, `u.bio`, `u.avatar`, `u.banner`, `u.twitter`, `u.discord`, `u.telegram`, `u.github`, `u.linkedin`)

4. **Check deactivation conditions.** The DID Document is marked as deactivated if any of the following are true:
   - The NFD has expired (`i.expirationTime` is in the past)
   - The NFD is not owned (the owner address equals the NFD Application's own address, indicating it has reverted to the contract)
   - The NFD is listed for sale (`i.sellamt` is non-zero)
   - The owner has explicitly deactivated the DID (`u.deactivated` is `"true"`)

   If deactivated, a minimal DID Document is returned containing only `@context` and `id`, with `didDocumentMetadata.deactivated` set to `true`.

5. **Build verification methods from the owner address.** Decode the Algorand owner address (`i.owner`) to extract the raw 32-byte Ed25519 public key (see [Section 10](#10-algorand-address-to-ed25519-key-pipeline)). Encode it as a `publicKeyMultibase` value using base58btc with the Ed25519 multicodec prefix (`0xed01`). Create a `VerificationMethod` with:
   - `id`: `did:nfd:<name>#owner`
   - `type`: `Ed25519VerificationKey2020`
   - `controller`: the DID itself (or the value of `u.controller` if set)
   - `publicKeyMultibase`: the multibase-encoded key

   Add this verification method to both `authentication` and `assertionMethod`.

6. **Derive X25519 key for key agreement.** Convert the owner's Ed25519 public key to an X25519 public key using the birational equivalence between the Ed25519 and Curve25519 curves (Montgomery form conversion). Encode it with the X25519 multicodec prefix (`0xec01`). Create a `KeyAgreement` entry with:
   - `id`: `did:nfd:<name>#x25519-owner`
   - `type`: `X25519KeyAgreementKey2020`

7. **Build verification methods from verified Algorand addresses (`v.caAlgo`).** Parse the comma-delimited list of verified Algorand addresses. For each address that is not a duplicate of the owner address, create an additional `VerificationMethod` with:
   - `id`: `did:nfd:<name>#algo-<index>`
   - `type`: `Ed25519VerificationKey2020`

8. **Parse additional keys from `u.keys`.** If the user-defined property `u.keys` contains valid JSON, parse it as an array of additional `VerificationMethod` objects. Ensure all IDs are properly prefixed with the DID and default the `controller` to the DID itself if not specified.

9. **Build service endpoints.** If `u.service` contains valid JSON, parse it as an array of `Service` objects. Ensure all IDs are properly prefixed with the DID. Then determine the `#web` LinkedDomains service URL using strict priority: `v.domain` (verified) > `u.website` > `u.url`. If any of these is set, create (or replace) the `#web` LinkedDomains service with that URL, overriding any `#web` entry from `u.service`.

10. **Build the `#profile` NFDProfile service.** If any of the profile properties (`u.name`, `u.bio`, `u.avatar`, `u.banner`) are present, create an `NFDProfile` service with `id: "#profile"`. The `serviceEndpoint` is a structured object with `name`, `bio`, `avatar`, and `banner` fields. For `avatar` and `banner`, verified properties (`v.avatar`, `v.banner`) take priority over user-defined (`u.avatar`, `u.banner`). If all four fields are empty, no `#profile` service is created. The `#profile` service is skipped if a service with that ID already exists in `u.service`.

11. **Build SocialMedia services.** For each supported social media platform, check for a handle in verified (`v.*`) or user-defined (`u.*`) properties (verified takes priority). If a handle is found, create a `SocialMedia` service with the platform's URL. If the handle already contains the platform URL prefix, it is used as-is; otherwise, the handle is formatted into the URL template. Services are skipped if a service with the same ID already exists in `u.service`.

    | Platform | NFD Key | Fragment | URL Template |
    |----------|---------|----------|-------------|
    | Twitter/X | `twitter` | `#twitter` | `https://x.com/{handle}` |
    | Discord | `discord` | `#discord` | `https://discord.com/users/{handle}` |
    | Telegram | `telegram` | `#telegram` | `https://t.me/{handle}` |
    | GitHub | `github` | `#github` | `https://github.com/{handle}` |
    | LinkedIn | `linkedin` | `#linkedin` | `https://linkedin.com/in/{handle}` |
    | Bluesky | `blueskydid` | `#bluesky` | `https://bsky.app/profile/{blueskydid}` |

    The final service ordering is: `#web` (LinkedDomains) → user-defined services from `u.service` → `#profile` (NFDProfile) → social media services (in platform table order).

12. **Build `alsoKnownAs`.** Collect alternative identifiers:
    - If `v.blueskydid` is set, add it as the first entry (this is a verified Bluesky DID).
    - If `u.alsoKnownAs` contains a valid JSON array of strings, append them.

13. **Set the controller.** The controller defaults to the DID itself (self-sovereign). If `u.controller` is set, its value overrides the default controller.

14. **Assemble the DID Document** with the standard JSON-LD contexts and all constructed elements.

### 7.2 Example DID Document

The following is a complete example of a DID Document resolved from `did:nfd:nfdomains.algo`:

```json
{
  "@context": [
    "https://www.w3.org/ns/did/v1",
    "https://w3id.org/security/suites/ed25519-2020/v1",
    "https://w3id.org/security/suites/x25519-2020/v1"
  ],
  "id": "did:nfd:nfdomains.algo",
  "controller": "did:nfd:nfdomains.algo",
  "verificationMethod": [
    {
      "id": "did:nfd:nfdomains.algo#owner",
      "type": "Ed25519VerificationKey2020",
      "controller": "did:nfd:nfdomains.algo",
      "publicKeyMultibase": "z6Mkf5rGMoatrSj1f4CyvuHBeXJELe9RPdzo2PKGNCKVtZxP"
    },
    {
      "id": "did:nfd:nfdomains.algo#algo-0",
      "type": "Ed25519VerificationKey2020",
      "controller": "did:nfd:nfdomains.algo",
      "publicKeyMultibase": "z6MkwFKiEbyLB7jrGMFJy4dDi3hMEMjQfMPBEB6zSWKx5Xqg"
    }
  ],
  "authentication": [
    "did:nfd:nfdomains.algo#owner"
  ],
  "assertionMethod": [
    "did:nfd:nfdomains.algo#owner"
  ],
  "keyAgreement": [
    {
      "id": "did:nfd:nfdomains.algo#x25519-owner",
      "type": "X25519KeyAgreementKey2020",
      "controller": "did:nfd:nfdomains.algo",
      "publicKeyMultibase": "z6LSbysY2xFMRpGMhb7tFTLMpeuPRaqaWM1yECx2AtzE3KCc"
    }
  ],
  "alsoKnownAs": [
    "did:plc:abc123xyz",
    "did:web:example.com"
  ],
  "service": [
    {
      "id": "did:nfd:nfdomains.algo#web",
      "type": "LinkedDomains",
      "serviceEndpoint": "https://nfdomains.algo.xyz"
    },
    {
      "id": "did:nfd:nfdomains.algo#messaging",
      "type": "DIDCommMessaging",
      "serviceEndpoint": "https://msg.example.com/patrick"
    },
    {
      "id": "did:nfd:nfdomains.algo#profile",
      "type": "NFDProfile",
      "serviceEndpoint": {
        "name": "NFDomains",
        "bio": "The naming identity layer for Algorand",
        "avatar": "https://images.nf.domains/avatar.png",
        "banner": "https://images.nf.domains/banner.png"
      }
    },
    {
      "id": "did:nfd:nfdomains.algo#twitter",
      "type": "SocialMedia",
      "serviceEndpoint": "https://x.com/naborhoods"
    },
    {
      "id": "did:nfd:nfdomains.algo#github",
      "type": "SocialMedia",
      "serviceEndpoint": "https://github.com/TxnLab"
    },
    {
      "id": "did:nfd:nfdomains.algo#bluesky",
      "type": "SocialMedia",
      "serviceEndpoint": "https://bsky.app/profile/did:plc:abc123xyz"
    }
  ]
}
```

### 7.3 Resolution Result

A full resolution result includes the DID Document along with resolution and document metadata:

```json
{
  "didDocument": { ... },
  "didResolutionMetadata": {
    "contentType": "application/did+json",
    "retrieved": "2025-02-13T12:00:00Z",
    "duration": 142
  },
  "didDocumentMetadata": {
    "created": "2024-03-15T12:00:00Z",
    "updated": "2025-01-10T08:30:00Z",
    "deactivated": false,
    "nfdAppId": 760937186
  }
}
```

### 7.4 Deactivated DID Document

When an NFD is expired, unowned, for sale, or explicitly deactivated, the resolver returns a minimal document:

```json
{
  "didDocument": {
    "@context": [
      "https://www.w3.org/ns/did/v1",
      "https://w3id.org/security/suites/ed25519-2020/v1",
      "https://w3id.org/security/suites/x25519-2020/v1"
    ],
    "id": "did:nfd:expired.algo"
  },
  "didResolutionMetadata": {
    "contentType": "application/did+json",
    "retrieved": "2025-02-13T12:00:00Z",
    "duration": 85
  },
  "didDocumentMetadata": {
    "deactivated": true,
    "nfdAppId": 55555
  }
}
```

---

## 8. CRUD Operations

### 8.1 Create

| Aspect | Details |
|--------|---------|
| **Method** | Register (mint) a new NFD via the NFD smart contract on Algorand. |
| **Process** | The user interacts with the NFD Registry contract to register a new `.algo` name. This involves submitting an Algorand transaction that creates a new Algorand Application (the NFD contract instance) and registers the name in the registry. |
| **On-chain effect** | A new Algorand Application is created with the registrant as the `i.owner`. The name-to-application mapping is stored in the registry's box storage. |
| **DID availability** | The `did:nfd:<name>.algo` identifier becomes resolvable immediately after the registration transaction is confirmed on the Algorand blockchain. |
| **Cost** | Registration requires Algorand transaction fees and an NFD registration fee (paid in ALGO). |
| **Tools** | [NFD App](https://app.nf.domains), NFD SDK, or direct Algorand transaction submission. |

No separate "DID registration" transaction is needed. The act of minting an NFD implicitly creates the DID.

### 8.2 Read (Resolve)

| Aspect | Details |
|--------|---------|
| **Method** | Query the Algorand blockchain via an algod node and dynamically construct the DID Document. |
| **Process** | 1. Parse and validate the DID string. 2. Look up the NFD Application ID from the registry. 3. Fetch the application's global state and box storage. 4. Construct the DID Document from the retrieved properties. |
| **Input** | A valid `did:nfd:<name>.algo` identifier. |
| **Output** | A `ResolutionResult` containing the DID Document, resolution metadata, and document metadata. |
| **Caching** | The resolver implementation uses an LRU cache with a configurable TTL (default: 5 minutes) to reduce blockchain query load. |
| **Content types** | `application/did+json` (default), `application/did+ld+json` (when JSON-LD processing is requested via the `Accept` header). |
| **Error codes** | `invalidDid` (malformed DID), `notFound` (NFD does not exist), `internalError` (blockchain query failure). |
| **HTTP endpoint** | `GET /1.0/identifiers/{did}` on the DID resolver HTTP service. |

### 8.3 Update

| Aspect | Details |
|--------|---------|
| **Method** | Submit Algorand transactions to modify NFD properties. |
| **Process** | The NFD owner (or authorized manager) submits application call transactions to the NFD smart contract, updating user-defined (`u.*`) properties in box storage. |
| **Updatable properties** | `u.service` (service endpoints), `u.keys` (additional verification methods), `u.controller` (controller override), `u.alsoKnownAs` (alternative identifiers), `u.deactivated` (explicit deactivation flag), `u.name` / `u.bio` / `u.avatar` / `u.banner` (profile data), `u.twitter` / `u.discord` / `u.telegram` / `u.github` / `u.linkedin` (social media handles). |
| **On-chain effect** | The application's box storage is updated. The next resolution will reflect the changes (subject to resolver cache TTL). |
| **Authorization** | Only the NFD owner (the Algorand account in `i.owner`) or an authorized manager can update properties. This is enforced by the NFD smart contract. |
| **Versioning** | No explicit versioning is maintained. The DID Document always reflects the current on-chain state. Historical states can be reconstructed using an Algorand Indexer with historical data. |

### 8.4 Deactivate

| Aspect | Details |
|--------|---------|
| **Method** | NFD expiration, sale listing, ownership reversion, or explicit deactivation. |
| **Conditions** | A DID is considered deactivated when any of the following is true: |
| | - **Expiration:** `i.expirationTime` is in the past. NFDs have a finite registration period and must be renewed. |
| | - **Not owned:** The `i.owner` address equals the NFD Application's own Algorand address (`crypto.GetApplicationAddress(appID)`), indicating the NFD has reverted to the contract and has no external owner. |
| | - **For sale:** `i.sellamt` is non-zero, indicating the NFD is listed for sale. During a sale listing, the identity is in a transitional state and the DID is treated as deactivated. |
| | - **Explicit deactivation:** `u.deactivated` is set to `"true"` by the owner. |
| **Effect** | The resolver returns a minimal DID Document (only `@context` and `id`) with `didDocumentMetadata.deactivated` set to `true`. |
| **Reactivation** | Deactivation is reversible. Renewing an expired NFD, removing a sale listing, transferring ownership back to an external account, or setting `u.deactivated` to a value other than `"true"` will reactivate the DID. |

---

## 9. NFD Property Mapping

NFD properties are organized into three namespaces stored in the Algorand Application's global state and box storage:

- **`i.*` (Internal):** Set by the NFD smart contract. Cannot be modified directly by the owner.
- **`v.*` (Verified):** Set through verified on-chain proofs (e.g., proving ownership of an Algorand address).
- **`u.*` (User-defined):** Set by the NFD owner via application call transactions to box storage.

### 9.1 Derived Properties

These properties are automatically read from the NFD and used to construct the DID Document. They do not require any special configuration by the NFD owner.

| NFD Property | Namespace | DID Document Element | Description |
|-------------|-----------|---------------------|-------------|
| `i.name` | Internal | `id` | The NFD name. Used to construct the DID identifier (`did:nfd:<name>`). |
| `i.owner` | Internal | `verificationMethod[0]`, `authentication`, `assertionMethod` | The Algorand address of the NFD owner. Decoded to an Ed25519 public key and used as the primary verification method (`#owner`). Also used to derive the X25519 key agreement key (`#x25519-owner`). |
| `i.timeCreated` | Internal | `didDocumentMetadata.created` | Unix timestamp of NFD creation. Formatted as RFC 3339 (e.g., `2024-03-15T12:00:00Z`). |
| `i.timeChanged` | Internal | `didDocumentMetadata.updated` | Unix timestamp of the last NFD modification. Formatted as RFC 3339. |
| `i.expirationTime` | Internal | `didDocumentMetadata.deactivated` | Unix timestamp of NFD expiration. If in the past, the DID is deactivated. |
| `i.sellamt` | Internal | `didDocumentMetadata.deactivated` | Sale amount in microAlgos. If non-zero, the NFD is for sale and the DID is deactivated. |
| `v.caAlgo` | Verified | `verificationMethod[1..n]` | Comma-delimited list of verified Algorand addresses (decoded from packed 32-byte public keys in box storage). Each address becomes an additional Ed25519 verification method (`#algo-<index>`). Duplicate of the owner address is skipped. |
| `v.domain` | Verified | `service` (`#web`) | Verified domain URL. Highest priority source for the `#web` LinkedDomains service endpoint (priority: `v.domain` > `u.website` > `u.url` > `u.service`). |
| `v.blueskydid` | Verified | `alsoKnownAs[0]`, `service` (`#bluesky`) | Verified Bluesky DID (`did:plc:...`). Added as the first entry in `alsoKnownAs`. Also auto-generates a `#bluesky` SocialMedia service with URL `https://bsky.app/profile/{blueskydid}`. |
| `v.avatar` | Verified | `service` (`#profile`) | Verified avatar URL. Takes priority over `u.avatar` in the NFDProfile service endpoint. |
| `v.banner` | Verified | `service` (`#profile`) | Verified banner URL. Takes priority over `u.banner` in the NFDProfile service endpoint. |
| `v.twitter` | Verified | `service` (`#twitter`) | Verified Twitter/X handle. Takes priority over `u.twitter`. Auto-generates a SocialMedia service. |
| `v.discord` | Verified | `service` (`#discord`) | Verified Discord handle. Takes priority over `u.discord`. Auto-generates a SocialMedia service. |
| `v.telegram` | Verified | `service` (`#telegram`) | Verified Telegram handle. Takes priority over `u.telegram`. Auto-generates a SocialMedia service. |
| `v.github` | Verified | `service` (`#github`) | Verified GitHub handle. Takes priority over `u.github`. Auto-generates a SocialMedia service. |
| `v.linkedin` | Verified | `service` (`#linkedin`) | Verified LinkedIn handle. Takes priority over `u.linkedin`. Auto-generates a SocialMedia service. |
| `v.blueskydid` | Verified | `service` (`#bluesky`) | Verified Bluesky DID. In addition to being added to `alsoKnownAs`, also auto-generates a SocialMedia service with URL `https://bsky.app/profile/{blueskydid}`. |

### 9.2 Optional User-Defined Properties

These properties are set by the NFD owner in box storage (`u.*` namespace) to customize the DID Document. All are optional.

| NFD Property | Format | DID Document Element | Description |
|-------------|--------|---------------------|-------------|
| `u.website` | String (URL) | `service` (`#web`) | Creates a `#web` LinkedDomains service endpoint. Used when `v.domain` is not set. Takes priority over `u.url` and any `#web` entry in `u.service`. |
| `u.url` | String (URL) | `service` (`#web`) | Fallback for `#web` LinkedDomains service. Used when neither `v.domain` nor `u.website` is set. Takes priority over any `#web` entry in `u.service`. |
| `u.service` | JSON array | `service` | Array of service endpoint objects. Each object has `id` (fragment), `type`, and `serviceEndpoint` fields. Fragment IDs are automatically prefixed with the DID. A `#web` entry in this array is the lowest priority source for the `#web` LinkedDomains service. |
| `u.keys` | JSON array | `verificationMethod[n..]` | Array of additional verification method objects. Each has `id`, `type`, `controller`, and `publicKeyMultibase` fields. Allows the owner to add non-Algorand keys (e.g., secp256k1, P-256). |
| `u.controller` | String (DID) | `controller` | Overrides the default controller (which is the DID itself). Allows delegation of control to another DID (e.g., `did:nfd:admin.algo`). |
| `u.alsoKnownAs` | JSON array of strings | `alsoKnownAs[1..]` | Additional alternative identifiers. Appended after `v.blueskydid` (if present). Can include DIDs from other methods or URI identifiers. |
| `u.deactivated` | `"true"` or absent | `didDocumentMetadata.deactivated` | When set to `"true"`, explicitly deactivates the DID. The resolver returns a minimal document. |
| `u.name` | String | `service` (`#profile`) | Display name for the NFDProfile service. |
| `u.bio` | String | `service` (`#profile`) | Bio/description for the NFDProfile service. |
| `u.avatar` | String (URL) | `service` (`#profile`) | Avatar image URL. Used in NFDProfile if `v.avatar` is not set. |
| `u.banner` | String (URL) | `service` (`#profile`) | Banner image URL. Used in NFDProfile if `v.banner` is not set. |
| `u.twitter` | String (handle) | `service` (`#twitter`) | Twitter/X handle. Auto-generates a SocialMedia service if `v.twitter` is not set. |
| `u.discord` | String (handle) | `service` (`#discord`) | Discord handle. Auto-generates a SocialMedia service if `v.discord` is not set. |
| `u.telegram` | String (handle) | `service` (`#telegram`) | Telegram handle. Auto-generates a SocialMedia service if `v.telegram` is not set. |
| `u.github` | String (handle) | `service` (`#github`) | GitHub handle. Auto-generates a SocialMedia service if `v.github` is not set. |
| `u.linkedin` | String (handle) | `service` (`#linkedin`) | LinkedIn handle. Auto-generates a SocialMedia service if `v.linkedin` is not set. |

### 9.3 Multi-Account Identity Model

The `did:nfd` method provides a multi-account identity model that is unique among DID methods. Rather than binding a single key or account to a single identifier, NFDs support a provable, on-chain many-to-many relationship between identifiers and Algorand accounts.

#### Forward Resolution: DID → Accounts

Resolving `did:nfd:nfdomains.algo` returns a DID Document containing verification methods for:

- The **owner address** (`i.owner`) — the primary Ed25519 key (`#owner`)
- All **verified linked addresses** (`v.caAlgo`) — additional Ed25519 keys (`#algo-0`, `#algo-1`, etc.)

Each verified address is a distinct Algorand account whose owner has cryptographically proven their association with the NFD through an on-chain transaction. This is not self-asserted metadata — the NFD smart contract verifies that the private key holder of each linked address explicitly authorized the linkage.

#### Reverse Resolution: Account → DIDs

Given any Algorand address, the NFD platform can return all NFDs where that address appears as:

- The **owner** (`i.owner`)
- A **verified linked address** (`v.caAlgo`)

This enables the reverse question: *"For this Algorand account, which DIDs reference it?"* The answer is a set of zero or more `did:nfd:*` identifiers, each backed by on-chain proof that the account is associated with the corresponding NFD.

#### Many-to-Many Relationship

The combination of forward and reverse resolution creates a true many-to-many relationship:

- **One NFD → many accounts:** A single NFD (and its `did:nfd` identifier) can have the owner address plus an arbitrary number of verified linked addresses.
- **One account → many NFDs:** A single Algorand account can be the owner of multiple NFDs and/or a verified linked address on additional NFDs.

Both directions are publicly verifiable on the Algorand blockchain. No trusted intermediary or off-chain attestation is required.

#### On-Chain Verification

The verified addresses in `v.caAlgo` are stored as packed 32-byte Ed25519 public keys in the NFD smart contract's box storage. The linkage is established through an on-chain opt-in transaction signed by the linked account's private key, providing the following security guarantees:

- **Proof of possession:** The linked account's owner provably authorized the association.
- **Tamper evidence:** The linkage transaction is recorded on the Algorand blockchain and can be independently audited.
- **Revocability:** Linked accounts can be removed by the NFD owner, and the removal is also recorded on-chain.
- **Liveness:** The DID Document is constructed at resolution time from current blockchain state, so revoked or added linkages are immediately reflected.

#### Comparison with Other DID Methods

| Capability | did:nfd | did:ens | did:ethr | did:ion | did:web | did:key | did:pkh | did:btcr |
|------------|---------|---------|----------|---------|---------|---------|---------|----------|
| Multiple accounts per identifier | **Yes** — owner + verified addresses | Partial — multi-chain addresses | Limited — delegates only | Yes — multiple controllers | Limited — keys in JSON file | No — single key | No — single address | Limited — single UTXO |
| Reverse lookup (account → identifiers) | **Yes** — all linked NFDs | Partial — one primary name only | No | No | No | No | No | No |
| True many-to-many | **Yes** | No | No | No | No | No | No | No |
| On-chain proof of account linkage | **Yes** — contract-verified opt-in | Yes — registry records | Yes — registry events | Yes — anchored operations | No — web server trust | No — deterministic | Partial — derived from address | Yes — UTXO reference |
| Verified vs. self-asserted linkage | **Verified** — linked account must sign | N/A | N/A | N/A | Self-asserted | N/A | N/A | N/A |

The closest comparable system is ENS (`did:ens`), which supports reverse resolution (address → ENS name). However, ENS reverse resolution returns only **one primary name** per address (set by the address owner), and there is no mechanism to discover all ENS names associated with an address. NFD reverse lookups return **all** NFDs linked to an address — both as owner and as verified linked accounts — providing complete bidirectional discoverability.

### 9.4 Example: Setting Service Endpoints

To declare a website and a DIDComm messaging endpoint, the NFD owner would set `u.service` to:

```json
[
  {
    "id": "#web",
    "type": "LinkedDomains",
    "serviceEndpoint": "https://nfdomains.algo.xyz"
  },
  {
    "id": "#messaging",
    "type": "DIDCommMessaging",
    "serviceEndpoint": "https://msg.example.com/patrick"
  }
]
```

The resolver prepends the DID to each fragment ID, producing `did:nfd:nfdomains.algo#web` and `did:nfd:nfdomains.algo#messaging`.

---

## 10. Algorand Address to Ed25519 Key Pipeline

### 10.1 Pipeline Overview

Algorand accounts are based on Ed25519 key pairs. An Algorand address is a 58-character base32 encoding of the 32-byte Ed25519 public key concatenated with a 4-byte SHA-512/256 checksum. The `did:nfd` method exploits this structure to extract the cryptographic public key directly from the Algorand address, without any additional on-chain key registration.

```
Algorand Address (58-char base32)
        |
        v
  base32 decode (no padding)
        |
        v
  36 bytes = pubkey[32] + checksum[4]
        |
        v
  first 32 bytes = Ed25519 public key
        |
        v
  multicodec prefix 0xed01 + pubkey
        |
        v
  base58btc encode + 'z' prefix
        |
        v
  publicKeyMultibase value
```

### 10.2 Step-by-Step Conversion

**Step 1: Base32 Decode**

The Algorand address is a base32-encoded string using the standard RFC 4648 alphabet (uppercase A-Z, 2-7) with no padding. Decode it to obtain 36 raw bytes.

```
Input:  "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ"
Output: [0x00, 0x00, ..., 0x00] (32 bytes) + [checksum] (4 bytes)
```

**Step 2: Extract Ed25519 Public Key**

Take the first 32 bytes of the decoded output. These are the raw Ed25519 public key. The remaining 4 bytes are the checksum (the last 4 bytes of `SHA-512/256(pubkey)`), which is used for address integrity verification but is not needed for key extraction.

```
pubkey = decoded[0:32]   // 32-byte Ed25519 public key
```

**Step 3: Apply Multicodec Prefix**

Prepend the Ed25519 multicodec prefix (`0xed, 0x01`) to the raw public key to create a 34-byte multicodec-prefixed key.

```
data = [0xed, 0x01] + pubkey   // 34 bytes
```

**Step 4: Base58btc Encode with Multibase Prefix**

Encode the 34-byte value using base58btc (Bitcoin's base58 alphabet). Prepend the multibase prefix character `z` (which designates base58btc encoding).

```
publicKeyMultibase = "z" + base58btc(data)
```

**Result:** A string like `z6Mkf5rGMoatrSj1f4CyvuHBeXJELe9RPdzo2PKGNCKVtZxP` that can be used directly as the `publicKeyMultibase` value in a `Ed25519VerificationKey2020` verification method.

### 10.3 X25519 Key Derivation

For key agreement (encryption), the Ed25519 public key is converted to an X25519 public key using the birational equivalence between the Ed25519 (twisted Edwards) curve and Curve25519 (Montgomery) curve:

1. Parse the 32-byte Ed25519 public key as a point on the Edwards25519 curve.
2. Convert the point to Montgomery form using `point.BytesMontgomery()`.
3. The resulting 32 bytes are the X25519 public key.
4. Apply the X25519 multicodec prefix (`0xec, 0x01`) and encode as base58btc with the `z` multibase prefix.

This produces a `publicKeyMultibase` value for use with `X25519KeyAgreementKey2020`, enabling encrypted communication channels (e.g., DIDComm) without requiring separate key registration.

---

## 11. Security Considerations

### 11.1 Proof of Ownership

The `did:nfd` method derives its security from the Algorand blockchain's consensus mechanism and the NFD smart contract's access control logic. Only the Algorand account that owns an NFD (as recorded in `i.owner`) can modify the NFD's user-defined properties. Ownership is enforced by Algorand's Ed25519 transaction signature verification -- modifying an NFD requires a valid signature from the owner's private key.

### 11.2 Immutability and Auditability

All NFD property changes are recorded as Algorand transactions on the blockchain. While the `did:nfd` method resolves the *current* state (not a historical snapshot), the full history of changes is available through the Algorand blockchain and can be audited using an Algorand Indexer. This provides a tamper-evident audit trail for all DID Document modifications.

### 11.3 Key Rotation

The `did:nfd` method supports key rotation through NFD ownership transfer. When an NFD is transferred to a new Algorand account, the `i.owner` property changes, which automatically updates the primary verification method (`#owner`), authentication, assertion method, and key agreement keys in the DID Document. Additionally, the owner can update `v.caAlgo` (verified addresses) to rotate associated verification methods, and `u.keys` to rotate any additional keys.

### 11.4 Replay Protection

Algorand transactions include a validity window (first-valid and last-valid round numbers), which prevents replay attacks. Each transaction also includes a unique group ID or lease field when needed. The blockchain's consensus protocol ensures that each transaction is processed exactly once.

### 11.5 NFD Expiration

NFDs have a finite registration period defined by `i.expirationTime`. When an NFD expires, the `did:nfd` resolver automatically deactivates the DID by returning a minimal document with `didDocumentMetadata.deactivated = true`. This prevents stale identity documents from persisting after the NFD owner has lost control of the name. Relying parties MUST check the `deactivated` flag in the document metadata before trusting a DID Document.

### 11.6 For-Sale NFDs

When an NFD is listed for sale (`i.sellamt != 0`), the DID is deactivated. This is a critical security measure: during a sale, the identity is in a transitional state where ownership may change imminently. Resolving parties MUST NOT rely on identity assertions from an NFD that is listed for sale, as the current owner may not retain control after a sale completes.

### 11.7 Unowned NFDs

When an NFD's `i.owner` address equals the NFD Application's own Algorand address (computed as `crypto.GetApplicationAddress(appID)`), the NFD is considered unowned -- it has reverted to the smart contract itself. This state occurs when an NFD is released or has never been claimed. The DID is deactivated in this state.

### 11.8 Resolver Trust

The `did:nfd` resolver queries an Algorand algod node to retrieve blockchain state. The security of the resolution depends on the trustworthiness of the algod node. In production deployments, the resolver SHOULD connect to a trusted algod node (ideally one operated by the resolver operator or a reputable node provider). Running a local Algorand participation or relay node provides the highest assurance that the returned state is authentic.

### 11.9 Cache Invalidation

The resolver uses an LRU cache with a configurable TTL (default: 5 minutes) for performance. During this window, changes to on-chain properties will not be reflected in resolved documents. Relying parties with strong freshness requirements SHOULD use a resolver with a short cache TTL or request uncached resolution.

### 11.10 Smart Contract Security

The NFD smart contract has been in production on the Algorand mainnet since 2022. The contract enforces access control for property modifications and ownership transfers. However, as with any smart contract, potential vulnerabilities in the contract logic could affect the security guarantees of the `did:nfd` method. The NFD smart contract is maintained by TxnLab Inc.

### 11.11 Verified Address Linkage

The `v.caAlgo` verified addresses provide cryptographically strong account linkage. Each address in `v.caAlgo` was added through an on-chain transaction signed by the corresponding Algorand account's private key, proving that the account holder explicitly authorized the association with the NFD. This design ensures that:

- **No impersonation is possible.** An NFD owner cannot claim an association with an Algorand account without that account's explicit on-chain consent.
- **Linkage is publicly auditable.** Any party can verify the existence and provenance of a verified address linkage by querying the Algorand blockchain directly.
- **Revocation is immediate.** When a verified address is removed from an NFD, the next DID resolution will reflect the change (subject to resolver cache TTL). The revocation transaction is also recorded on-chain for auditability.
- **Reverse lookups are trustworthy.** Because the linkage is contract-verified in both directions, reverse resolution (address → NFDs) produces results that are as trustworthy as forward resolution (NFD → addresses). Relying parties can trust that every NFD returned by a reverse lookup has a genuine, authorized relationship with the queried account.

---

## 12. Privacy Considerations

### 12.1 Public Blockchain

All NFD properties used to construct the DID Document are stored on the Algorand public blockchain. This means that the DID Document contents -- including the owner's Algorand address, verified addresses, service endpoints, and alternative identifiers -- are publicly visible to anyone who queries the blockchain. DID subjects SHOULD be aware that any information placed in NFD properties is permanently recorded on a public ledger.

### 12.2 No Personally Identifiable Information

The `did:nfd` method does not inherently require or store personally identifiable information (PII). The primary identifier is a pseudonymous NFD name, and the verification methods are derived from Algorand addresses (which are pseudonymous public keys). However, NFD owners may choose to associate their NFD with real-world identifiers via service endpoints or `alsoKnownAs` entries. Owners SHOULD carefully consider the privacy implications before linking their DID to identifiable information.

### 12.3 Correlation Risk

Because Algorand addresses and NFD names are public, it is possible to correlate a `did:nfd` identifier with:
- All Algorand transactions associated with the owner's address
- Other NFDs owned by the same Algorand account
- Verified addresses listed in `v.caAlgo`
- Bluesky accounts linked via `v.blueskydid`
- Service endpoints declared in `u.service`

DID subjects who require strong pseudonymity SHOULD use dedicated Algorand accounts for their NFD ownership and be selective about which verified addresses and service endpoints they associate with their NFD.

### 12.4 Service Endpoints and Off-Chain Data

Service endpoints declared in `u.service` point to off-chain resources controlled by the DID subject. The `did:nfd` method does not make any guarantees about the privacy, availability, or integrity of data hosted at service endpoints. Relying parties SHOULD apply appropriate security measures when interacting with service endpoints.

### 12.5 Herd Privacy

The `did:nfd` method benefits from the large number of NFDs registered on the Algorand blockchain. Resolving a `did:nfd` identifier requires querying standard Algorand application state, which is indistinguishable from any other application state query. An algod node operator can observe which NFDs are being resolved, but this is mitigated by running a local algod node or using a trusted node provider.

### 12.6 Right to Be Forgotten

Due to the append-only nature of the Algorand blockchain, historical NFD property values cannot be deleted. While the current DID Document reflects only the latest state (and deactivated NFDs return minimal documents), the historical record of all property changes remains on the blockchain. DID subjects SHOULD be aware that data written to NFD properties cannot be retroactively removed.

---

## 13. References

### Normative References

- [W3C Decentralized Identifiers (DIDs) v1.0](https://www.w3.org/TR/did-core/) -- The core specification for Decentralized Identifiers.
- [DID Resolution v1.0](https://w3c-ccg.github.io/did-resolution/) -- Specification for DID resolution, dereferencing, and metadata.
- [Ed25519 Signature Suite 2020](https://w3c-ccg.github.io/di-eddsa-2020/) -- Defines the `Ed25519VerificationKey2020` verification method type.
- [X25519 Key Agreement Key 2020](https://w3id.org/security/suites/x25519-2020) -- Defines the `X25519KeyAgreementKey2020` key agreement type.
- [Multibase Data Format](https://datatracker.ietf.org/doc/html/draft-multiformats-multibase) -- Specification for multibase-prefixed binary-to-text encoding.
- [Multicodec](https://github.com/multiformats/multicodec) -- Self-describing codecs for binary data. Ed25519 public key: `0xed`, X25519 public key: `0xec`.

### Informative References

- [Algorand Developer Documentation](https://developer.algorand.org/) -- Documentation for the Algorand blockchain platform.
- [NFD (Non-Fungible Domains)](https://app.nf.domains) -- The NFD application for registering and managing `.algo` domains.
- [RFC 4648 -- Base Encodings](https://datatracker.ietf.org/doc/html/rfc4648) -- Defines base32 encoding used by Algorand addresses.
- [RFC 8032 -- Edwards-Curve Digital Signature Algorithm (EdDSA)](https://datatracker.ietf.org/doc/html/rfc8032) -- Defines the Ed25519 signature scheme used by Algorand.
- [RFC 7748 -- Elliptic Curves for Security](https://datatracker.ietf.org/doc/html/rfc7748) -- Defines X25519 key agreement used for `keyAgreement`.
- [DID Specification Registries](https://www.w3.org/TR/did-spec-registries/) -- W3C registry of DID methods, verification method types, and service types.
