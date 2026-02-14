# DID:NFD User Guide

## Introduction

Every Algorand NFD (Non-Fungible Domain) is also a **Decentralized Identifier (DID)** -- a globally unique, self-sovereign identity that you control with your Algorand wallet.

DIDs are a [W3C standard](https://www.w3.org/TR/did-core/) for decentralized digital identity. They let you prove who you are without relying on a central authority like Google, Facebook, or a certificate issuer. Instead of a username on someone else's platform, a DID is an identifier *you* own, backed by cryptographic keys *you* hold.

The `did:nfd` method bridges Algorand NFDs into the DID ecosystem:

```
did:nfd:nfdomains.algo
```

That single string is a resolvable identifier. Anyone can look it up and get back a **DID Document** containing your public keys, service endpoints, and linked identities -- all sourced from your on-chain NFD data.

**Why does this matter?**

- **Self-sovereign identity**: Your Algorand wallet key proves you own the DID. No passwords, no third parties.
- **Human-readable**: Unlike `did:key:z6Mk...` or `did:plc:abc123`, `did:nfd:nfdomains.algo` is something you can actually read and remember.
- **Zero setup for basics**: If you own an NFD, you already have a DID. Your owner key is automatically included.
- **Interoperable**: Works with the broader W3C DID ecosystem -- verifiable credentials, decentralized authentication, cross-chain identity linking.
- **On-chain and verifiable**: The source of truth is the Algorand blockchain, not a centralized server.

---

## Quick Start

Resolve any NFD as a DID with a single HTTP request:

```bash
curl http://localhost:8080/1.0/identifiers/did:nfd:nfdomains.algo
```

You will get back a JSON response containing the full DID Document:

```json
{
  "didDocument": {
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
        "publicKeyMultibase": "z6Mkf5r..."
      }
    ],
    "authentication": ["did:nfd:nfdomains.algo#owner"],
    "assertionMethod": ["did:nfd:nfdomains.algo#owner"],
    "keyAgreement": [
      {
        "id": "did:nfd:nfdomains.algo#x25519-owner",
        "type": "X25519KeyAgreementKey2020",
        "controller": "did:nfd:nfdomains.algo",
        "publicKeyMultibase": "z6LShs..."
      }
    ]
  },
  "didResolutionMetadata": {
    "contentType": "application/did+json",
    "retrieved": "2025-12-01T12:00:00Z",
    "duration": 42
  },
  "didDocumentMetadata": {
    "deactivated": false,
    "nfdAppId": 12345
  }
}
```

That is it. Your NFD is now a globally resolvable decentralized identity.

---

## Your DID Automatically

Here is the good news: **if you own an NFD, you already have a DID.** No configuration required.

Every NFD has an owner -- an Algorand address, which is an Ed25519 public key. The DID resolver automatically:

1. Reads your owner address from the blockchain
2. Converts it to a standard Ed25519 verification method
3. Derives an X25519 key for encrypted communication (key agreement)
4. Builds a complete DID Document

So `did:nfd:yourname.algo` resolves immediately for any active, owned NFD. The basic document includes:

- **Your owner key** as a verification method (`#owner`)
- **Authentication** capability (prove you are the NFD owner)
- **Assertion** capability (sign statements/credentials)
- **Key agreement** capability (establish encrypted channels)
- **Verified Algorand addresses** from `v.caAlgo` (if you have verified additional wallets)
- **Bluesky DID** from `v.blueskydid` in `alsoKnownAs` (if you have linked your Bluesky account)
- **NFD Profile** as a `#profile` service (if you have set a name, bio, avatar, or banner on your NFD)
- **Social media links** as individual services (if you have set Twitter, Discord, Telegram, GitHub, or LinkedIn handles)

You only need to configure additional properties if you want to add custom service endpoints, extra keys, or other advanced features.

---

## Multi-Account Identity

One of the most powerful features of NFD DIDs is **multi-account identity** — the ability to provably link multiple Algorand wallets to a single NFD, and to look up any wallet address to find all the NFDs it is associated with.

### Your NFD links multiple wallets

If you have verified additional Algorand addresses on your NFD (through the NFD app's "Verified Addresses" feature), those accounts automatically appear in your DID Document as verification methods:

```json
{
  "verificationMethod": [
    {
      "id": "did:nfd:nfdomains.algo#owner",
      "type": "Ed25519VerificationKey2020",
      "publicKeyMultibase": "z6Mkf5r...",
      "blockchainAccountId": "AAAA...YYYY"
    },
    {
      "id": "did:nfd:nfdomains.algo#algo-0",
      "type": "Ed25519VerificationKey2020",
      "publicKeyMultibase": "z6Mkx9a...",
      "blockchainAccountId": "BBBB...ZZZZ"
    },
    {
      "id": "did:nfd:nfdomains.algo#algo-1",
      "type": "Ed25519VerificationKey2020",
      "publicKeyMultibase": "z6Mkw2b...",
      "blockchainAccountId": "CCCC...WWWW"
    }
  ]
}
```

Each of these keys is a fully functional Ed25519 verification method. A verifier resolving your DID can see every Algorand account you have provably linked to your identity.

### Reverse lookups — find DIDs from any address

NFD supports reverse resolution: given any Algorand address, you can discover all NFDs where that address appears as owner or as a verified linked account. This means:

- **Forward lookup:** *"What accounts does `did:nfd:nfdomains.algo` reference?"* → The owner address plus all verified linked addresses.
- **Reverse lookup:** *"For this Algorand account, what DIDs reference it?"* → All NFDs where this account is the owner or a verified linked address.

This bidirectional resolution creates a true **many-to-many** relationship:

- One NFD can link to many accounts (owner + verified addresses)
- One account can be linked to many NFDs (as owner of some, verified address on others)
- Both directions are publicly verifiable on the Algorand blockchain

### Why this is unique

Most DID methods bind a single key or account to a single identifier:

| Method | Account model | Reverse lookup? |
|--------|--------------|-----------------|
| `did:key` | One key = one DID | No |
| `did:pkh` | One address = one DID | No |
| `did:ethr` | One Ethereum address = one DID | No |
| `did:web` | One domain = one DID (keys in JSON file) | No |
| `did:ion` | Multiple controllers, but no reverse discovery | No |
| `did:ens` | One name, reverse returns one primary name only | Partial (one name per address) |
| **`did:nfd`** | **Owner + verified linked accounts** | **Yes — all linked NFDs** |

ENS is the closest comparison — it supports reverse resolution (address → ENS name), but only returns **one primary name** per address. NFD reverse lookups return **all** NFDs associated with an address, providing complete bidirectional discoverability.

Critically, the linkage is not self-asserted. Each verified address on an NFD was added through an on-chain transaction signed by that account's private key. The NFD smart contract enforces this — you cannot claim a wallet is linked to your NFD unless that wallet's owner proves it on-chain.

### Practical use cases

- **Personal identity consolidation.** Link your hardware wallet, hot wallet, and multisig to a single NFD. Anyone receiving funds from any of those wallets can verify they all belong to `did:nfd:yourname.algo`.
- **Organizational identity.** A company NFD can link team member wallets. A counterparty can verify that an unfamiliar address is associated with a known organization.
- **Cross-account verification.** Prove that multiple accounts operating across different dApps belong to the same identity, without revealing your private keys.
- **Account discovery.** Given an unknown address, check if it is linked to any NFD — turning a raw public key into a human-readable, verifiable identity.

---

## Setting Up DID Properties on Your NFD

To customize your DID Document beyond the basics, you can set user-defined properties on your NFD. These are stored in the `u.*` namespace and can be updated by the NFD owner through standard NFD update transactions.

The following properties are recognized by the DID resolver:

| Property | Type | Purpose |
|----------|------|---------|
| `u.website` | string (URL) | Creates a `#web` LinkedDomains service (priority: `v.domain` > `u.website` > `u.url` > `u.service`) |
| `u.url` | string (URL) | Fallback for `#web` LinkedDomains if `v.domain` and `u.website` are not set |
| `u.service` | JSON array | Custom service endpoints (websites, messaging, etc.) |
| `u.keys` | JSON array | Additional verification methods (non-Algorand keys) |
| `u.controller` | string | Override the DID controller |
| `u.alsoKnownAs` | JSON array | Link to other identifiers you control |
| `u.deactivated` | string | Set to `"true"` to deactivate the DID |
| `u.name` | string | Display name (auto-generates `#profile` NFDProfile service) |
| `u.bio` | string | Bio/description (auto-generates `#profile` NFDProfile service) |
| `u.avatar` | string (URL) | Avatar image URL (used in `#profile` if `v.avatar` not set) |
| `u.banner` | string (URL) | Banner image URL (used in `#profile` if `v.banner` not set) |
| `u.twitter` | string (handle) | Twitter/X handle (auto-generates `#twitter` SocialMedia service) |
| `u.discord` | string (handle) | Discord handle (auto-generates `#discord` SocialMedia service) |
| `u.telegram` | string (handle) | Telegram handle (auto-generates `#telegram` SocialMedia service) |
| `u.github` | string (handle) | GitHub handle (auto-generates `#github` SocialMedia service) |
| `u.linkedin` | string (handle) | LinkedIn handle (auto-generates `#linkedin` SocialMedia service) |

You set these properties using an NFD update transaction on the Algorand blockchain (e.g., through the NFD app at [app.nf.domains](https://app.nf.domains) or programmatically via the Algorand SDK).

### Website / Domain

The resolver automatically creates a `#web` LinkedDomains service endpoint using strict priority:

1. **`v.domain`** (verified) -- highest priority, cryptographically proven
2. **`u.website`** -- user-set website URL
3. **`u.url`** -- user-set URL (fallback)
4. **`u.service` `#web` entry** -- lowest priority

The first non-empty value wins. If you have a verified domain (`v.domain`), no additional configuration is needed. Otherwise, the simplest option is to set `u.website` to your website URL:

```
https://yourname.algo.xyz
```

### Service Endpoints (u.service)

Service endpoints tell others how to interact with you. Common examples include your website, messaging service, or API endpoint.

Set `u.service` to a JSON array of service objects:

```json
[
  {
    "id": "#web",
    "type": "LinkedDomains",
    "serviceEndpoint": "https://yourname.algo.xyz"
  }
]
```

> **Note:** If `v.domain`, `u.website`, or `u.url` is set, they take priority for the `#web` LinkedDomains service over any `#web` entry in `u.service`.

Each service object has three fields:

| Field | Description |
|-------|-------------|
| `id` | A fragment identifier (starts with `#`). The resolver prefixes it with your DID automatically. |
| `type` | The service type. Common values: `LinkedDomains`, `DIDCommMessaging`, `CredentialRegistry`. |
| `serviceEndpoint` | The URL where the service is accessible. |

**Multiple services:**

```json
[
  {
    "id": "#web",
    "type": "LinkedDomains",
    "serviceEndpoint": "https://yourname.algo.xyz"
  },
  {
    "id": "#messaging",
    "type": "DIDCommMessaging",
    "serviceEndpoint": "https://msg.example.com/yourname"
  }
]
```

### NFD Profile (auto-generated)

If you have set any of the profile properties on your NFD (`u.name`, `u.bio`, `u.avatar`, `u.banner`), the resolver automatically creates a `#profile` service of type `NFDProfile`. No JSON configuration is needed -- just set the properties on your NFD.

For avatar and banner, verified properties (`v.avatar`, `v.banner`) take priority over user-defined values.

The resulting service in your DID Document looks like:

```json
{
  "id": "did:nfd:yourname.algo#profile",
  "type": "NFDProfile",
  "serviceEndpoint": {
    "name": "Your Name",
    "bio": "Your bio text",
    "avatar": "https://example.com/avatar.png",
    "banner": "https://example.com/banner.png"
  }
}
```

Only non-empty fields are included. If all four fields are empty, no `#profile` service is created.

### Social Media (auto-generated)

Social media handles stored on your NFD are automatically converted to `SocialMedia` services. For each platform, verified (`v.*`) handles take priority over user-defined (`u.*`).

| NFD Property | Service ID | URL Format |
|-------------|-----------|------------|
| `twitter` | `#twitter` | `https://x.com/{handle}` |
| `discord` | `#discord` | `https://discord.com/users/{handle}` |
| `telegram` | `#telegram` | `https://t.me/{handle}` |
| `github` | `#github` | `https://github.com/{handle}` |
| `linkedin` | `#linkedin` | `https://linkedin.com/in/{handle}` |
| `blueskydid` | `#bluesky` | `https://bsky.app/profile/{blueskydid}` |

For example, if `u.twitter` is set to `myhandle`, your DID Document will include:

```json
{
  "id": "did:nfd:yourname.algo#twitter",
  "type": "SocialMedia",
  "serviceEndpoint": "https://x.com/myhandle"
}
```

If you define a custom service with the same ID (e.g., `#twitter`) in `u.service`, the auto-generated version is skipped and your custom definition is preserved.

### Additional Keys (u.keys)

If you have non-Algorand keys you want to include in your DID Document (for example, an Ethereum key or a separate signing key), set `u.keys` to a JSON array of verification method objects:

```json
[
  {
    "id": "#key-2",
    "type": "Ed25519VerificationKey2020",
    "publicKeyMultibase": "z6MkhaXg..."
  }
]
```

Each key object supports:

| Field | Description |
|-------|-------------|
| `id` | A fragment identifier (starts with `#`). Auto-prefixed with your DID. |
| `type` | Key type, e.g., `Ed25519VerificationKey2020`, `EcdsaSecp256k1VerificationKey2019`. |
| `controller` | Who controls this key. Defaults to your DID if omitted. |
| `publicKeyMultibase` | The public key in multibase (base58btc, `z` prefix) encoding. |

These keys are appended to the verification methods in your DID Document, alongside the automatically derived owner key.

### Controller Override (u.controller)

By default, your DID is its own controller (self-sovereign). If you want to delegate control to another DID -- for example, an organizational DID that manages multiple identities -- set `u.controller`:

```
did:nfd:admin.algo
```

This changes the `controller` field in your DID Document from `did:nfd:yourname.algo` to the specified DID.

### Also Known As (u.alsoKnownAs)

Link your DID to other identifiers you control. Set `u.alsoKnownAs` to a JSON array of URIs:

```json
[
  "did:web:example.com",
  "https://twitter.com/yourhandle"
]
```

Note: If you have a verified Bluesky DID (`v.blueskydid`), it is automatically included in `alsoKnownAs` -- you do not need to add it manually.

---

## Understanding the DID Document

Here is a walkthrough of every field in a fully populated DID Document:

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
      "publicKeyMultibase": "z6Mkf5r..."
    },
    {
      "id": "did:nfd:nfdomains.algo#algo-0",
      "type": "Ed25519VerificationKey2020",
      "controller": "did:nfd:nfdomains.algo",
      "publicKeyMultibase": "z6Mkx9a..."
    }
  ],
  "authentication": ["did:nfd:nfdomains.algo#owner"],
  "assertionMethod": ["did:nfd:nfdomains.algo#owner"],
  "keyAgreement": [
    {
      "id": "did:nfd:nfdomains.algo#x25519-owner",
      "type": "X25519KeyAgreementKey2020",
      "controller": "did:nfd:nfdomains.algo",
      "publicKeyMultibase": "z6LShs..."
    }
  ],
  "service": [
    {
      "id": "did:nfd:nfdomains.algo#web",
      "type": "LinkedDomains",
      "serviceEndpoint": "https://nfdomains.algo.xyz"
    },
    {
      "id": "did:nfd:nfdomains.algo#profile",
      "type": "NFDProfile",
      "serviceEndpoint": {
        "name": "NFDomains",
        "bio": "The naming identity layer for Algorand",
        "avatar": "https://images.nf.domains/avatar.png"
      }
    },
    {
      "id": "did:nfd:nfdomains.algo#twitter",
      "type": "SocialMedia",
      "serviceEndpoint": "https://x.com/naborhoods"
    },
    {
      "id": "did:nfd:nfdomains.algo#bluesky",
      "type": "SocialMedia",
      "serviceEndpoint": "https://bsky.app/profile/did:plc:abc123xyz"
    }
  ],
  "alsoKnownAs": [
    "did:plc:abc123xyz",
    "did:web:example.com"
  ]
}
```

**Field-by-field breakdown:**

| Field | Source | Description |
|-------|--------|-------------|
| `@context` | Automatic | JSON-LD contexts defining the vocabulary. Always includes DID v1, Ed25519, and X25519 suites. |
| `id` | Automatic | Your DID, derived from your NFD name: `did:nfd:<name>.algo`. |
| `controller` | `u.controller` or automatic | Who controls this DID. Defaults to the DID itself (self-sovereign). |
| `verificationMethod` | Owner address + `v.caAlgo` + `u.keys` | All public keys associated with this DID. The `#owner` key is always first. Verified Algorand addresses from `v.caAlgo` appear as `#algo-0`, `#algo-1`, etc. |
| `authentication` | Automatic | Which keys can authenticate (prove identity). Only the owner key. |
| `assertionMethod` | Automatic | Which keys can make assertions (sign credentials). Only the owner key. |
| `keyAgreement` | Automatic | X25519 key derived from the owner's Ed25519 key, used for encrypted communication. |
| `service` | `v.domain` / `u.website` / `u.url` / `u.service` / profile & social props | Service endpoints. Includes: `#web` LinkedDomains (priority: `v.domain` > `u.website` > `u.url` > `u.service`), user-defined services from `u.service`, auto-generated `#profile` NFDProfile (from `u.name`/`u.bio`/`u.avatar`/`u.banner`), and auto-generated SocialMedia services (from twitter/discord/telegram/github/linkedin handles). |
| `alsoKnownAs` | `v.blueskydid` + `u.alsoKnownAs` | Other identifiers linked to this DID. Bluesky DID is included first if present. |

---

## Proving Ownership (Challenge-Response)

Your DID is backed by your Algorand Ed25519 key. To prove you control `did:nfd:yourname.algo`, you sign a challenge with the private key corresponding to your NFD's owner address.

### How it works

1. A verifier sends you a challenge (a random nonce or message).
2. You sign it with your Algorand wallet's Ed25519 private key (the key behind your NFD's owner address).
3. The verifier resolves your DID, extracts the `#owner` verification method, and verifies the signature.

### Example flow

**Step 1: Verifier creates a challenge**
```
Challenge: "authenticate-did:nfd:nfdomains.algo-1701432000-abc123"
```

**Step 2: You sign the challenge**

Using your Algorand wallet (or SDK), sign the challenge bytes with the Ed25519 key for your NFD owner address. In the Algorand Go SDK, this looks like:

```go
import "github.com/algorand/go-algorand-sdk/v2/crypto"

// Sign the challenge with your account's private key
signature := crypto.SignBytes(privateKey, []byte(challenge))
```

**Step 3: Verifier checks the signature**

The verifier resolves `did:nfd:nfdomains.algo`, extracts the `publicKeyMultibase` from the `#owner` verification method, decodes it back to a raw Ed25519 public key, and verifies:

```go
import "crypto/ed25519"

// Decode the multibase key from the DID Document
pubkey := decodeMultibase(didDoc.VerificationMethod[0].PublicKeyMultibase)

// Verify the signature
valid := ed25519.Verify(pubkey, []byte(challenge), signature)
```

If `valid` is `true`, the signer controls `did:nfd:nfdomains.algo`.

### Key encoding details

Algorand addresses are base32-encoded Ed25519 public keys (32 bytes key + 4 bytes checksum). The DID Document encodes these keys using the W3C standard format:

```
Raw Ed25519 key (32 bytes)
  -> prepend multicodec prefix (0xed, 0x01)
  -> base58btc encode
  -> prepend 'z' multibase prefix
  -> publicKeyMultibase value
```

---

## Running the Resolver

The DID resolver is a standalone HTTP server that resolves `did:nfd:*` identifiers by querying the Algorand blockchain.

### Docker deployment

Build the resolver image:

```bash
docker build -f did/Dockerfile -t nfd-did-resolver:latest .
```

Run the container:

```bash
docker run -d \
  -p 8080:8080 \
  -e ALGOD_URL=https://mainnet-api.4160.nodely.dev \
  -e ALGOD_TOKEN="" \
  -e REGISTRY_ID=760937186 \
  -e CACHE_TTL=5m \
  nfd-did-resolver:latest
```

### Running directly

```bash
go build -o did-resolver ./did/cmd/did-resolver
./did-resolver
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LISTEN` | `:8080` | Address and port to listen on |
| `ALGOD_URL` | `https://mainnet-api.4160.nodely.dev` | Algorand algod node URL |
| `ALGOD_TOKEN` | (empty) | Algod API authentication token |
| `REGISTRY_ID` | `760937186` | NFD Registry smart contract application ID |
| `CACHE_TTL` | `5m` | How long resolved documents are cached (Go duration format, e.g., `5m`, `10m`, `1h`) |

### Verifying it works

```bash
# Health check
curl http://localhost:8080/health

# Resolve a DID
curl http://localhost:8080/1.0/identifiers/did:nfd:nfdomains.algo

# Check method properties
curl http://localhost:8080/1.0/properties
```

---

## API Reference

### GET /1.0/identifiers/{did}

Resolve a DID to its DID Document.

**Request:**
```
GET /1.0/identifiers/did:nfd:nfdomains.algo
Accept: application/did+json
```

The `Accept` header is optional. Supported content types:
- `application/did+json` (default)
- `application/did+ld+json`

**Response (200 OK):**
```json
{
  "didDocument": { ... },
  "didResolutionMetadata": {
    "contentType": "application/did+json",
    "retrieved": "2025-12-01T12:00:00Z",
    "duration": 42
  },
  "didDocumentMetadata": {
    "deactivated": false,
    "nfdAppId": 12345
  }
}
```

**Response (410 Gone) -- deactivated DID:**
```json
{
  "didDocument": {
    "@context": ["https://www.w3.org/ns/did/v1", "..."],
    "id": "did:nfd:expired.algo"
  },
  "didResolutionMetadata": {
    "contentType": "application/did+json",
    "retrieved": "2025-12-01T12:00:00Z"
  },
  "didDocumentMetadata": {
    "deactivated": true,
    "nfdAppId": 55555
  }
}
```

### GET /health

Health check endpoint.

**Response (200 OK):**
```json
{
  "status": "ok"
}
```

### GET /1.0/properties

Returns metadata about the `did:nfd` method.

**Response (200 OK):**
```json
{
  "method": "nfd",
  "network": "algorand",
  "properties": {
    "identifierFormat": "did:nfd:<name>.algo",
    "identifierRegex": "^did:nfd:([a-z0-9]{1,27}\\.){1,2}algo$",
    "blockchain": "algorand",
    "keyType": "Ed25519",
    "supportsDeactivation": true,
    "supportsExpiration": true,
    "supportsKeyAgreement": true,
    "supportsServices": true
  }
}
```

### HTTP Status Codes

| Code | Meaning | When |
|------|---------|------|
| 200 | OK | DID resolved successfully |
| 400 | Bad Request | Invalid DID format (e.g., wrong method, segment name, uppercase) |
| 404 | Not Found | NFD does not exist on the blockchain |
| 410 | Gone | DID is deactivated (expired, for sale, unowned, or explicitly deactivated) |
| 500 | Internal Server Error | Blockchain query failed or unexpected error |

---

## DID Deactivation

A DID can become deactivated in several ways. When deactivated, the resolver returns a minimal DID Document with `deactivated: true` in the metadata and an HTTP 410 (Gone) status code.

### Automatic deactivation

These conditions are checked automatically at resolution time:

- **Expired NFD**: The NFD's `expirationTime` has passed. Renew your NFD to reactivate.
- **For-sale NFD**: The NFD has a non-zero `sellamt` (listed for sale). Remove the listing to reactivate.
- **Unowned NFD**: The NFD's owner is the contract itself (i.e., the NFD has been returned to the registry). Purchase the NFD to activate it.

### Explicit deactivation

You can explicitly deactivate your DID while keeping your NFD active by setting:

```
u.deactivated = "true"
```

This is useful if you want to signal that your DID should no longer be used for authentication or credential verification, without giving up the NFD itself. Remove the property (or set it to any value other than `"true"`) to reactivate.

### What a deactivated document looks like

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
    "retrieved": "2025-12-01T12:00:00Z"
  },
  "didDocumentMetadata": {
    "deactivated": true,
    "nfdAppId": 55555
  }
}
```

Notice: no verification methods, no services, no authentication. The DID exists but is inert.

---

## Bluesky Integration

If you have verified your Bluesky account with your NFD, the Bluesky DID (`v.blueskydid`) is automatically used in two ways:

1. **`alsoKnownAs`**: The Bluesky DID is added as the first entry in `alsoKnownAs`, creating a bidirectional link between your `did:nfd` identity and your Bluesky (`did:plc`) identity.
2. **`#bluesky` service**: A `SocialMedia` service is auto-generated with the URL `https://bsky.app/profile/{blueskydid}`, linking to your Bluesky profile.

For example, if your NFD has `v.blueskydid` set to `did:plc:abc123xyz`, your DID Document will include:

```json
{
  "alsoKnownAs": [
    "did:plc:abc123xyz"
  ],
  "service": [
    {
      "id": "did:nfd:yourname.algo#bluesky",
      "type": "SocialMedia",
      "serviceEndpoint": "https://bsky.app/profile/did:plc:abc123xyz"
    }
  ]
}
```

You do not need to add your Bluesky DID to `u.alsoKnownAs` -- it is picked up automatically from the verified property.

---

## Limitations

1. **Root and single-segment NFDs only**: Both root NFDs (`nfdomains.algo`) and single-segment NFDs (`mail.nfdomains.algo`) resolve as DIDs. Multi-level segments (e.g., `a.b.c.algo`) are not valid DIDs.

2. **Public blockchain data**: All DID Document data is sourced from the public Algorand blockchain. Do not store private or sensitive information in your NFD properties.

3. **Cache TTL**: The resolver caches resolved documents for the configured `CACHE_TTL` (default 5 minutes). Changes to your NFD properties on-chain may take up to this long to appear in resolved documents.

4. **No key rotation history**: The DID Document always reflects the current on-chain state. There is no built-in version history of previous keys or documents.

5. **Algorand key type**: The owner key is always Ed25519 (Algorand's native key type). Other key types can be added via `u.keys` but the primary verification method is always Ed25519.

6. **Resolver availability**: The DID resolver must be able to reach an Algorand algod node. If the node is unreachable, resolution will fail.

---

## Troubleshooting

### "invalidDid" error (400)

The DID format is wrong. Check that:
- The DID starts with `did:nfd:`
- The NFD name is all lowercase
- It is a root or single-segment NFD (multi-level segments like `a.b.c.algo` are not valid)
- The name ends with `.algo`
- Each label is 1-27 alphanumeric characters

```bash
# Valid
curl http://localhost:8080/1.0/identifiers/did:nfd:nfdomains.algo

# Invalid -- uppercase
curl http://localhost:8080/1.0/identifiers/did:nfd:Nfdomains.algo

# Invalid -- multi-level segment
curl http://localhost:8080/1.0/identifiers/did:nfd:a.b.c.algo

# Invalid -- wrong method
curl http://localhost:8080/1.0/identifiers/did:web:nfdomains.algo
```

### "notFound" error (404)

The NFD does not exist on the blockchain. Verify the NFD exists:
- Check on [app.nf.domains](https://app.nf.domains)
- Ensure you are querying the correct network (mainnet vs testnet)
- Confirm the `REGISTRY_ID` environment variable matches your target network

### Deactivated document (410)

The DID exists but is deactivated. Check if the NFD is:
- **Expired**: Renew it at [app.nf.domains](https://app.nf.domains)
- **For sale**: Remove the sale listing
- **Unowned**: Purchase the NFD
- **Explicitly deactivated**: Remove the `u.deactivated` property

### Service endpoints or keys not appearing

- Verify your JSON is valid (use a JSON linter)
- Ensure `u.service` and `u.keys` are valid JSON arrays
- Check that the NFD update transaction was confirmed on-chain
- Wait for the cache TTL to expire (default 5 minutes), or restart the resolver

### Resolver not starting

- Verify the `ALGOD_URL` is reachable: `curl <ALGOD_URL>/v2/status`
- Check that the `ALGOD_TOKEN` is correct (can be empty for public nodes)
- Ensure the port specified in `LISTEN` is not in use

### Testing with curl

```bash
# Resolve a DID
curl -s http://localhost:8080/1.0/identifiers/did:nfd:nfdomains.algo | jq .

# Request JSON-LD format
curl -s -H "Accept: application/did+ld+json" \
  http://localhost:8080/1.0/identifiers/did:nfd:nfdomains.algo | jq .

# Health check
curl -s http://localhost:8080/health | jq .

# Method properties
curl -s http://localhost:8080/1.0/properties | jq .
```

### Verifying your NFD's DNS records (related)

If you also use NFD DNS and want to confirm your NFD is working on both fronts:

```bash
# DNS resolution
dig nfdomains.algo.xyz A

# DID resolution
curl http://localhost:8080/1.0/identifiers/did:nfd:nfdomains.algo
```

---

## Quick Reference

| I want to... | What to do |
|--------------|------------|
| Resolve my DID | `curl http://localhost:8080/1.0/identifiers/did:nfd:yourname.algo` |
| Add a website service | Verify your domain (`v.domain`), or set `u.website` to `https://yoursite.com` |
| Add my profile | Set `u.name`, `u.bio`, `u.avatar`, `u.banner` on your NFD -- `#profile` service is auto-generated |
| Add social media links | Set `u.twitter`, `u.github`, etc. on your NFD -- SocialMedia services are auto-generated |
| Add extra keys | Set `u.keys` to a JSON array of verification method objects |
| Link my Bluesky | Verify Bluesky through NFD -- it is automatic |
| Link other identities | Set `u.alsoKnownAs` to a JSON array of URIs |
| Delegate control | Set `u.controller` to another DID string |
| Deactivate my DID | Set `u.deactivated` to `"true"` |
| Check resolver health | `curl http://localhost:8080/health` |
