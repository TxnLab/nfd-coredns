# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

nfd-coredns is a CoreDNS plugin that resolves DNS queries for Algorand NFDs (Non-Fungible Domains) by querying blockchain data. It bridges blockchain-based naming to standard DNS queries, allowing domains like `patrick.algo.xyz` to resolve via DNS.

## Build & Test Commands

```bash
# Build
go build -v ./...

# Build with Git commit info (production)
go build -v -o out/ -ldflags="-s -w -X github.com/coredns/coredns/coremain.GitCommit=$(git describe --dirty --always)" .

# Run all tests
go test -v ./...

# Run tests in specific package
go test -v ./internal/nfd

# Run a specific test
go test -v -run TestGetNfdRRs ./internal/nfd

# Docker build
docker buildx build --platform linux/amd64 -t nfddns:latest .
```

## Architecture

### Request Flow
1. DNS query arrives (e.g., `patrick.algo.xyz`)
2. CoreDNS `rewrite` plugin strips `.xyz` suffix → `patrick.algo`
3. NFD plugin checks LRU cache for DNS records
4. If cache miss, fetches NFD data from Algorand blockchain via algod client
5. Converts NFD JSON DNS properties to standard DNS RR format
6. Returns DNS response (or delegates to file plugin for root zones / forwarder for external domains)

### Key Files

| File | Purpose |
|------|---------|
| `main.go` | Registers CoreDNS plugins and directive order |
| `setup.go` | Plugin initialization, config parsing, embedded zones |
| `nfd.go` | Main plugin handler implementing `plugin.Handler` |
| `internal/nfd/nfdrr.go` | `NfdRRHandler` - manages NFD lookups with LRU caching |
| `internal/nfd/fetch.go` | `NfdFetcher` - queries Algorand blockchain for NFD data |
| `internal/nfd/dnsjson.go` | Converts NFD JSON DNS data to DNS RR records |
| `internal/zones/` | Embedded root zone file for algo.xyz |

### Key Interfaces

```go
// NfdRRHandler - manages lookup and caching of NFD DNS records
type NfdRRHandler interface {
    GetNfdRRs(ctx context.Context, log clog.P, qname string) ([]JsonRr, error)
}

// NfdFetcher - fetches NFD properties from Algorand blockchain
type NfdFetcher interface {
    FetchNfdDnsVals(ctx context.Context, names []string) (map[string]Properties, error)
}
```

### Plugin Chain
`nfd` → `file` (embedded zones) → `forward` (Cloudflare 1.1.1.1)

## Configuration (Corefile)

```
algo.xyz {
  nfd {
    node https://mainnet-api.4160.nodely.dev  # Algorand algod URL (required)
    token <token>                              # algod auth token (can be empty)
    registryid 760937186                       # NFD Registry contract ID
    cachemins 5                                # LRU cache TTL in minutes
    algoxyzip 34.111.170.195                   # Default A record IP
  }
}
```

## Supported DNS Record Types
A, AAAA, CNAME, MX, TXT, SRV, CERT, NS, SOA, CAA

## Rewrite Rule and NS Records

The Corefile uses a `rewrite` rule to convert external queries:
- External: `patrick.algo.xyz.` → Internal: `patrick.algo.`
- Responses are converted back automatically

**NS record handling:**
- NS records only exist at zone apex (`algo.xyz.`)
- NFD subdomains are NOT delegated zones and have no NS records
- NS queries for NFDs return NODATA with SOA in authority (per RFC 2308)
- The embedded zone files (`internal/zones/`) define the authoritative NS/SOA

## Key Errors (internal/nfd)
- `ErrNfdNotFound` - NFD doesn't exist on blockchain
- `ErrNfdTooManySegments` - Query exceeds segment depth limits
- `ErrNfdSplitOwnership` - Segment owned by different account
- `ErrNfdExpired` - NFD registration has expired