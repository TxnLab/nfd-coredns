# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

nfd-coredns is a CoreDNS plugin that resolves DNS queries for Algorand NFDs (Non-Fungible Domains) by querying blockchain data. It bridges blockchain-based naming to standard DNS, allowing domains like `patrick.algo.xyz` to resolve via any resolver.

## Companion docs — keep these in sync

This file is deliberately lean. Two other docs carry the prose, and they are the ones that
go stale:

| Doc | Covers | Update it when... |
|-----|--------|-------------------|
| `README.md` | Architecture diagram, request flow, V2/V3 contract handling, the full Corefile options table, project structure | Config directives, defaults, plugin wiring, or contract-version behavior change |
| `docs/NFD_DNS_USER_GUIDE.md` | End-user guide for authoring DNS records on an NFD: record format, supported types, name/`@` scoping rules, TTL bounds, segments, Bluesky, IPFS DNSLink | Record handling, name scoping, TTL rules, supported qtypes, segment behavior, or any user-visible response code changes |

**Do not duplicate the Corefile options table here** — point at README instead. That
duplication is what caused the last round of drift.

`docs/psl_update_docs.md` is an untracked internal runbook for the Public Suffix List
submission for `algo.xyz` (see the `_psl` TXT record in `internal/zones/algo.xyz`).

## Build & Test Commands

Requires **Go 1.26**.

```bash
# Build
go build -v ./...

# Run all tests
go test -v ./...

# Run tests in specific package
go test -v ./internal/nfd

# Run a specific test
go test -v -run TestGetNfdRRs ./internal/nfd

# Production build (note the jsonv2 experiment tag — used by Dockerfile and release.yaml)
go build -v -tags=goexperiment.jsonv2 -o out/ \
  -ldflags="-s -w -X github.com/coredns/coredns/coremain.GitCommit=$(git describe --dirty --always)" .

# Docker build
docker buildx build --platform linux/amd64 -t nfddns:latest .
```

CI (`.github/workflows/go.yml`) runs only `go build -v ./...` and `go test -v ./...` — no
lint or vet step, so don't assume a linter will catch anything.

## Architecture

### Request Flow

1. DNS query arrives (e.g., `patrick.algo.xyz`)
2. CoreDNS `rewrite` plugin strips the `.xyz` suffix → `patrick.algo`
3. NFD plugin checks its LRU caches for NFD properties / DNS records
4. On cache miss, fetches NFD data from the Algorand blockchain via the algod client
5. Converts NFD JSON DNS properties to standard DNS RR format
6. Returns the response, or falls through to the `file` plugin (root zones) / forwarder (external names)

### Plugin wiring (easy to get wrong)

`nfd` is the **last** entry in `dnsserver.Directives` (`main.go:25-40`). The chain is not a
flat list:

- `NfdPlugin.Next` **is** the `file` plugin, holding the embedded zones (`setup.go:90-95`).
- `NfdPlugin.Forwarder` is a **separate** `forward` handler pointed at `1.1.1.1:53`
  (`setup.go:99-101`), invoked selectively through a `nonwriter` in `LookupViaForwarder`
  (`nfd.go:332-346`) — it is *not* chained after `file`.

### Key Files

| File | Purpose |
|------|---------|
| `main.go` | CoreDNS plugin imports and `dnsserver.Directives` order |
| `setup.go` | Plugin registration, Corefile parsing, `//go:embed internal/zones`, handler wiring |
| `nfd.go` | `NfdPlugin` — `ServeDNS`/`Lookup`/`Query`; the supported-qtype allow-list lives here |
| `Corefile` | Example config: **two** server blocks, `algo.xyz` (mainnet) and `dotalgo.io` (testnet, registry `84366825`) |
| `internal/nfd/nfdrr.go` | `NfdRRHandler` — root/segment resolution, merge-vs-delegate, two-tier LRU cache |
| `internal/nfd/fetch.go` | `NfdFetcher` — algod queries, registry box/LSIG lookup, TEAL state decode, expiry/ownership/version checks |
| `internal/nfd/dnsjson.go` | `JsonRr` ⇄ `dns.RR` conversion, `ConvertOriginRefs` scope enforcement, `MergeJsonRrrs` |
| `internal/nfd/misc.go` | `isValidNFDName` — `a-z0-9` only, 1-27 chars/label, max one segment level |
| `internal/zones/` | **Two** embedded zone files: `algo.xyz` and `dotalgo.io` |

### Key Interfaces

These are the only two interfaces in the repo.

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

## Configuration (Corefile)

Directives: `node` (required), `token`, `registryid`, `algoxyzip`, `cachemins`. Parsed in
`nfdParse` (`setup.go:130-219`); each rejects zero or multiple values, and `algoxyzip` is
validated as IPv4.

Defaults are `setup.go:33-38` — **not** the values in the example `Corefile`:

```go
defRegId        = 760937186
defAlgoXyzIp    = "34.8.101.7"   // Corefile example uses 34.111.170.195
defCacheMinutes = 1              // minutes, not 5
```

See README.md for the full options table with descriptions.

## Invariants & Gotchas

Behavior that is easy to break and not obvious from a single file:

1. **The supported-qtype allow-list is in `nfd.go`, not `dnsjson.go`.** There are two
   mirrored `switch qtype` fallthrough chains — the admission gate in `Lookup`
   (`nfd.go:155-178`) and the dispatch in `Query` (`nfd.go:306-329`). **Edit both together.**
   `DnsRRsFromJsonRRs` is entirely type-agnostic: it composes a zone-file line and calls
   `dns.NewRR`. Current set: SOA, NS, CAA, CNAME, TXT, MX, SRV, CERT, A, AAAA. Anything
   else returns **NOTIMP** (`nfd.go:115-116`), including HTTPS/SVCB and ANY.

2. **NS is intercepted before record matching** (`nfd.go:229-238`). NFD subdomains are not
   delegated zones, so a user-stored NS record is never servable — it only counts toward
   the name-exists check. NS returns NODATA/NXDOMAIN with the zone SOA per RFC 2308.

3. **NXDOMAIN vs NODATA** hinges on `NameExistsInJsonRRs` (`nfd.go:284-297`,
   `dnsjson.go:70-77`): name has records of *another* type → NODATA (NOERROR); name has no
   records at all → NXDOMAIN. Both carry the SOA from `zoneSOA`, extracted from the server
   block's own embedded zone at `setup.go:83-87`.

4. **`_`-prefixed labels in the NFD-name position are never NFD lookups.** `nfd.go:202-204`
   returns `Result.Delegation` for names like `_psl.algo` / `_acme-challenge.algo`, which
   `ServeDNS` (`nfd.go:76-89`) rewrites back to the zone origin and hands to the `file`
   plugin. This is *not* a DNS NS referral. Note `_dnslink.patrick.algo` is unaffected —
   the check looks at the second-to-last label.

5. **"Delegated segment" means NFD ownership, not DNS delegation.** `nfdrr.go:131-160`: a
   segment owned by a different account serves *only* its own records; a same-owner segment
   merges with the root, root winning on name+type conflict (`MergeJsonRrrs`,
   `dnsjson.go:188-206`). Either way a flat `[]JsonRr` is returned and no NS referral is
   ever emitted.

6. **`ConvertOriginRefs` rewrites `Name` only — never `RrData`** (`dnsjson.go:143-186`).
   Consequences: (a) every record is force-rooted under the owning NFD's FQDN, so
   out-of-scope and cross-root names get re-rooted rather than served; (b) `@` in *rdata*
   is **not** an origin reference — it reaches `dns.NewRR` verbatim and resolves against
   origin `.`, so `"rrData": ["@"]` silently becomes a CNAME to the DNS root. rdata must be
   a fully-qualified name.

7. **Name matching is exact — there is no wildcard support.** `dnsjson.go:90` uses
   `strings.EqualFold` on the full name, so a stored `*.@` matches only the literal name
   `*.<nfd>.`, never arbitrary subdomains.

8. **The synthetic-placeholder rule is broader than "expired."** `fetch.go:128-143`
   overwrites `UserDefined["dns"]` with a single apex A record pointing at `algoxyzip`
   whenever the NFD is expired, **is listed for sale** (`sellamt != 0`, `fetch.go:482-492`),
   is owned by its own app account, or simply has no `dns` property. So any existing NFD
   always answers A at its apex — and listing an NFD for sale silently disables all of its
   DNS records.

9. **Segment depth violations return SERVFAIL, not NXDOMAIN.** `ErrNfdTooManySegments`
   (`nfdrr.go:91-95`) is not `ErrNfdNotFound`, so it falls into the generic error branch at
   `nfd.go:218-221`. The depth budget is 4 labels after discounting the *leading contiguous
   run* of `_`-prefixed labels (`nfdrr.go:87-91`).

10. **CNAMEs are chased for every qtype** (`nfd.go:242-273`), internally for `*.algo`
    targets and via the 1.1.1.1 forwarder otherwise, with both the CNAME and the resolved
    answer placed in ANSWER. A side effect is that apex CNAMEs work, unlike in normal DNS.

11. **One unparseable rdata takes down the whole name.** `DnsRRsFromJsonRRs` returns on the
    first `dns.NewRR` failure (`dnsjson.go:114-117`) → SERVFAIL (`nfd.go:277-283`). Because
    of the unconditional CNAME pre-query, a malformed CNAME breaks *every* qtype at that name.

12. **Negative results are cached too.** Not-found NFDs are stored as an empty `Properties{}`
    (`nfdrr.go:196-207`), so a newly minted NFD stays NXDOMAIN for up to one `cachemins`
    period on top of the CoreDNS `cache` plugin and downstream resolvers.

13. **Only definitive not-found results may be negatively cached.** `fetchNFDs` bails out
    before the negative-cache loop when the fetch failed with anything other than
    `ErrNfdNotFound` (`nfdrr.go:206-214`). Caching a transient algod failure as not-found used
    to convert a momentary blip into sustained SERVFAILs for a whole `cachemins` period —
    including for healthy delegated segments whose root happened to share the failed batch.
    Keep that guard ahead of the loop when touching this function.

14. **A missing root in the fetch result means NXDOMAIN, not SERVFAIL.** `GetNfdRRs` returns
    `ErrNfdNotFound` when the root NFD is absent from `nfdData` (`nfdrr.go:108-119`), because
    a negatively-cached root makes `fetchNFDs` return `(results-without-the-root, nil)` — no
    error. Returning a generic error there would surface as SERVFAIL and make a non-existent
    NFD answer NXDOMAIN on the first query but SERVFAIL on every cached repeat. The separate
    name-mismatch check below it is a real data-integrity error and *should* stay generic.
    Note both checks guard the root *fetch result*, not the root's RRs — commit `2a65b83`'s
    "skip building and validating the root's RRs" for delegated segments is accurate and
    unrelated; these checks simply run earlier. Regression tests:
    `TestGetNfdRRs_NotFoundIsStableAcrossCacheHits` and
    `TestGetNfdRRs_TransientFetchErrorIsNotNegativelyCached`.

## Supported DNS Record Types

A, AAAA, CNAME, MX, TXT, SRV, CERT, CAA, NS, SOA — but see gotchas 1 and 2: NS is never
servable from NFD data, and SOA is accepted as a qtype and *can* be matched from user
records, which is an oddity rather than a feature.

TTL is clamped to 60-86400s, defaulting to 300 (`dnsjson.go:22-26`). A zero, absent, or
**negative** TTL yields 300 — negatives bypass the clamp rather than being raised to 60.

## Rewrite Rule and NS Records

The Corefile uses a `rewrite` rule to convert external queries:
- External: `patrick.algo.xyz.` → Internal: `patrick.algo.`
- Responses are converted back automatically

`rewriteToZoneOrigin` (`nfd.go:125-144`) does the reverse for fall-through: it maps the
internal `*.algo` form back to the server block's `zoneOrigin` so the `file` plugin can
match its embedded zone and produce a proper NXDOMAIN+SOA.

## Errors

| Error | Where | Notes |
|-------|-------|-------|
| `ErrNfdNotFound` | `fetch.go:36` | Mapped to NoData → fall-through → NXDOMAIN from the zone file |
| `ErrNFdIncompatible` | `fetch.go:37` | Pre-v3 contract carrying `dns`/`blueskydid` → SERVFAIL (note the capital-F typo in the name) |
| `ErrNfdExpired` | `fetch.go:38` | **Declared but never referenced** — expiry is handled via the `IsNFdExpired` boolean |
| `ErrNfdNotOwned` | `fetch.go:39` | **Declared but never referenced** — same, via `IsNfdOwned` |
| `ErrNfdTooManySegments` | `nfdrr.go:25` | Depth limit → SERVFAIL |
| `ErrInvalidDNSJson` | `dnsjson.go:19` | Unmarshal failure of the on-chain `dns` property |
| `errNotImplemented` | `nfd.go:55` | Unexported, package `main`; unsupported qtype → NOTIMP |

## Tests

Six test files, all table-driven with `t.Run`, using testify (`assert`/`require`) except
`misc_test.go`. Mocks are hand-written locally — `mockNfdRRHandler`, `mockForwarder`,
`mockNextPlugin`, `testResponseWriter` in `nfd_test.go`; `mockNfdFetcher` in
`nfdrr_test.go`. No testdata dirs, no golden files, no mock framework, no `go:generate`.
