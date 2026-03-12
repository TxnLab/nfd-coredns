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

A, AAAA, CNAME, MX, TXT, SRV, CAA, CERT, NS, SOA

TTL values are clamped between 60 and 86,400 seconds (default: 300s).

## NFD Features

- **Segments**: NFDs support subdomains (segments) up to 3 labels deep (e.g., `mail.patrick.algo`). Segment ownership is validated against the root NFD owner.
- **Bluesky integration**: If an NFD has a verified Bluesky DID, the plugin automatically generates an `_atproto` TXT record.
- **Expiration handling**: Expired NFDs return a default A record pointing to a configurable IP address.
- **Two-level caching**: NFD properties and DNS RR sets are cached separately in TTL-based LRU caches (50K entries each).

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
        cachemins 5                                 # LRU cache TTL in minutes
        algoxyzip 34.111.170.195                    # Default A record IP for expired NFDs
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
| `cachemins` | No | `5` | Cache TTL in minutes |
| `algoxyzip` | No | `34.8.101.7` | Default A record IP for expired/unowned NFDs |

## Building

```bash
# Standard build
go build -v ./...

# Production build with version info
go build -v -o out/ \
  -ldflags="-s -w -X github.com/coredns/coredns/coremain.GitCommit=$(git describe --dirty --always)" .

# Docker (linux/amd64)
docker buildx build --platform linux/amd64 -t nfddns:latest .
```

Requires **Go 1.25+**.

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
├── Corefile                 # Example CoreDNS configuration
├── Dockerfile               # Multi-stage Docker build (golang → scratch)
├── internal/
│   ├── nfd/
│   │   ├── nfdrr.go         # NfdRRHandler — lookup orchestration with LRU caching
│   │   ├── fetch.go         # NfdFetcher — Algorand blockchain queries
│   │   ├── dnsjson.go       # JSON DNS record → DNS RR conversion
│   │   └── misc.go          # NFD name validation
│   └── zones/
│       └── algo.xyz          # Embedded root zone file for algo.xyz
└── docs/
    └── NFD_DNS_USER_GUIDE.md # User guide for configuring DNS records on NFDs
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

The CoreDNS plugin chain processes requests in this order:

`rewrite` → `nfd` → `cache` → `file` (embedded zones) → `forward` (Cloudflare DNS)

- **rewrite**: Strips/restores the `.xyz` TLD suffix
- **nfd**: Resolves NFD names from blockchain data
- **file**: Serves root zone queries (NS, SOA) from embedded zone files
- **forward**: Resolves external CNAME targets via Cloudflare `1.1.1.1`

## Zone

The plugin serves the `algo.xyz` zone, compiled into the binary via `go:embed`.

## License

MIT — see [LICENSE](LICENSE) for details.
