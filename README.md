# nfd-coredns

A [CoreDNS](https://coredns.io/) plugin that resolves DNS queries for [Algorand NFDs](https://app.nf.domains) (Non-Fungible Domains) by querying on-chain data. It bridges blockchain-based naming to standard DNS, allowing domains like `patrick.algo.xyz` to resolve via any DNS resolver.

## How It Works

NFDs store DNS records as JSON on the Algorand blockchain. This plugin fetches those records in real time and serves them as standard DNS responses.

```
                         ┌──────────────┐
 dig patrick.algo.xyz    │   CoreDNS    │
─────────────────────►   │              │
                         │  rewrite     │  patrick.algo.xyz → patrick.algo
                         │  ▼           │
                         │  nfd plugin  │──── cache miss ───► Algorand blockchain
                         │  ▼           │◄── NFD properties ─┘
                         │  file plugin │  (embedded zone for root zone queries)
                         │  ▼           │
◄────────────────────    │  forward     │  (Cloudflare 1.1.1.1 for external CNAMEs)
   DNS response          └──────────────┘
```

### Request Flow

1. DNS query arrives (e.g., `patrick.algo.xyz A`)
2. CoreDNS `rewrite` plugin strips the `.xyz` suffix → `patrick.algo`
3. NFD plugin checks its LRU cache for DNS records
4. On cache miss, queries the Algorand blockchain via algod for the NFD's on-chain DNS properties
5. Converts the NFD's JSON DNS data to standard DNS resource records
6. Returns the DNS response (rewrite restores the `.xyz` suffix)

Root zone queries (e.g., `algo.xyz NS`) are served from embedded zone files. Out-of-zone CNAME targets are resolved via a Cloudflare DNS forwarder.

## Supported DNS Record Types

A, AAAA, CNAME, MX, TXT, SRV, CAA, CERT, NS, SOA — any other qtype (including HTTPS/SVCB
and ANY) returns NOTIMP.

`NS` is accepted as a query type but is never answered from NFD data: NFD subdomains are
not delegated zones, so NS queries return NODATA or NXDOMAIN with the zone's SOA in the
authority section, per RFC 2308.

TTL values are clamped between 60 and 86,400 seconds (default: 300s). A zero, absent, or
negative TTL yields the 300s default rather than being clamped to the 60s minimum.

Name matching is exact and case-insensitive — **wildcard records are not supported**.

## NFD Features

- **Segments**: NFDs support subdomains (segments), e.g. `relay.belt.algo`. A segment is its own NFD and always serves its own DNS records. When the segment shares the root NFD's owner, the root can extend it with sub-records (root wins on conflict); when it's owned by a different account, only that segment's own records are served for its subtree — an NFD-ownership boundary, where the plugin still answers authoritatively (no DNS NS delegation — see the Limitations section of [the user guide](docs/NFD_DNS_USER_GUIDE.md)).
- **Bluesky integration**: If an NFD has a verified Bluesky DID, the plugin automatically generates an `_atproto` TXT record.
- **Placeholder fallback**: An NFD returns a single apex A record pointing at the configurable `algoxyzip` address when it is expired, **listed for sale**, owned by its own app account, or simply has no `dns` property configured. Listing an NFD for sale therefore disables all of its DNS records.
- **Two-level caching**: NFD properties and DNS RR sets are cached separately in TTL-based LRU caches (50K entries each).

### V2 vs V3 Contract Handling

NFDs exist at different smart contract versions on the Algorand blockchain. The plugin handles them differently:

- **V3+ NFDs** (contract version ≥ 3.0) can store explicit DNS records as JSON in their on-chain properties. When present, these are parsed and served as real DNS responses (A, AAAA, CNAME, MX, etc.).

- **V2 NFDs** do not support explicit DNS records. Instead, the plugin returns a synthetic A record pointing to the `algoxyzip` IP address (default: `34.8.101.7`). A web service at that IP handles HTTP redirects — if the NFD owner has configured a forwarding URL, the service redirects there; otherwise it redirects to the NFD's landing page on `app.nf.domains`.

- **Expired or unowned V3 NFDs** are treated the same as V2 — their explicit DNS properties are ignored and the fallback A record redirect is returned instead.

- **V2 NFDs with V3-only properties** (e.g., `dns` or `blueskydid` set on a pre-V3 contract) are flagged as incompatible and return an error.

The registry lookup itself also has version tiers: the plugin first attempts a V2 box-based lookup, falling back to the legacy V1 logic-signature approach if the box isn't found.

## Configuration

The plugin is configured in a CoreDNS `Corefile`:

```
algo.xyz {
    rewrite name regex (.*)\.algo\.xyz {1}.algo
    rewrite answer name (.*)\.algo {1}.algo.xyz

    nfd {
        node https://mainnet-api.4160.nodely.dev   # Algorand algod API URL (required)
        token ""                                    # algod auth token (optional)
        registryid 760937186                        # NFD Registry app ID
        cachemins 5                                 # LRU cache TTL in minutes (overrides the default of 1)
        algoxyzip 34.111.170.195                    # A record IP for placeholder responses (overrides 34.8.101.7)
    }

    cache {
        keepttl
    }

    forward . 1.1.1.1
}
```

### Configuration Options

| Option | Required | Default | Description |
|--------|----------|---------|-------------|
| `node` | Yes | — | Algorand algod API endpoint URL |
| `token` | No | `""` | Algod authentication token |
| `registryid` | No | `760937186` | NFD Registry smart contract application ID |
| `cachemins` | No | `1` | Cache TTL in minutes |
| `algoxyzip` | No | `34.8.101.7` | Default A record IP for expired/unowned NFDs |

## Building

```bash
# Standard build
go build -v ./...

# Production build with version info
# (the goexperiment.jsonv2 tag matches the Dockerfile and release workflow)
go build -v -tags=goexperiment.jsonv2 -o out/ \
  -ldflags="-s -w -X github.com/coredns/coredns/coremain.GitCommit=$(git describe --dirty --always)" .

# Docker (linux/amd64)
docker buildx build --platform linux/amd64 -t nfddns:latest .
```

Requires **Go 1.26+**.

## Testing

```bash
# Run all tests
go test -v ./...

# Run tests for a specific package
go test -v ./internal/nfd

# Run a specific test
go test -v -run TestGetNfdRRs ./internal/nfd
```

## Project Structure

```
├── main.go                  # CoreDNS plugin registration and directive ordering
├── nfd.go                   # NfdPlugin handler (ServeDNS, Lookup, Query)
├── setup.go                 # Plugin initialization and Corefile config parsing
├── Corefile                 # Example CoreDNS configuration (algo.xyz + dotalgo.io blocks)
├── Dockerfile               # Multi-stage Docker build (golang → scratch)
├── CLAUDE.md                # Repo guidance for Claude Code, incl. invariants and gotchas
├── internal/
│   ├── nfd/
│   │   ├── nfdrr.go         # NfdRRHandler — lookup orchestration with LRU caching
│   │   ├── fetch.go         # NfdFetcher — Algorand blockchain queries
│   │   ├── dnsjson.go       # JSON DNS record → DNS RR conversion
│   │   └── misc.go          # NFD name validation
│   └── zones/
│       ├── algo.xyz         # Embedded root zone file for algo.xyz (mainnet)
│       └── dotalgo.io       # Embedded root zone file for dotalgo.io (testnet)
└── docs/
    ├── NFD_DNS_USER_GUIDE.md # User guide for configuring DNS records on NFDs
    └── psl_update_docs.md    # Runbook for the algo.xyz Public Suffix List submission
```

## Key Interfaces

```go
// NfdRRHandler manages lookup and caching of NFD DNS records
type NfdRRHandler interface {
    GetNfdRRs(ctx context.Context, log clog.P, qname string) ([]JsonRr, error)
}

// NfdFetcher queries the Algorand blockchain for NFD properties
type NfdFetcher interface {
    FetchNfdDnsVals(ctx context.Context, names []string) (map[string]Properties, error)
}
```

## Plugin Chain

Requests are processed in this order:

`rewrite` → `nfd` → `file` (embedded zones)

- **rewrite**: Strips/restores the `.xyz` TLD suffix
- **nfd**: Resolves NFD names from blockchain data
- **file**: Serves root zone queries (NS, SOA) from embedded zone files

Two details the arrow diagram hides. `nfd` is the **last** entry in `dnsserver.Directives`,
and the `file` plugin is wired directly as `NfdPlugin.Next` rather than sitting later in a
flat chain. The Cloudflare forwarder is **not** in the chain at all — it is a separate
handler on `NfdPlugin.Forwarder`, called selectively through a `nonwriter` when an
out-of-zone name (typically a CNAME target) has to be resolved.

## Zone

The plugin serves the `algo.xyz` zone, compiled into the binary via `go:embed`.

## License

MIT — see [LICENSE](LICENSE) for details.
