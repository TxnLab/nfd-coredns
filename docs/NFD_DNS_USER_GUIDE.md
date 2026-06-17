# NFD DNS User Guide

## Introduction

NFD (Non-Fungible Domains) brings the power of decentralized naming to standard DNS. When you own an NFD like `patrick.algo`, you can configure DNS records that are stored permanently on the Algorand blockchain and served as live DNS zone data.

**Your NFD becomes a real, working domain:**
- `patrick.algo.xyz` - accessible via standard DNS

This means you can point your NFD to a website, receive email, configure SSL certificates, and more - all with records you control on-chain.

## How It Works

```
1. You configure DNS records in your NFD (stored on Algorand blockchain)
        ↓
2. The NFD DNS service reads your on-chain data
        ↓
3. Standard DNS queries resolve your NFD as a normal domain
```

When someone queries `patrick.algo.xyz`, the NFD DNS service fetches your records from the blockchain and returns a standard DNS response. No special software needed - it just works with any browser or application.

## DNS Record Format

DNS records are stored as JSON in your NFD. Each record has these fields:

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Where the record applies (use `@` for your domain) |
| `type` | Yes | Record type: A, AAAA, CNAME, MX, TXT, SRV, CAA |
| `rrData` | Yes | Array of record values |
| `ttl` | No | Cache time in seconds (default: 300) |

### Name Field

The `name` field says *where* the record applies inside your NFD's DNS zone. **All records are rooted under your NFD** — you have DNS authority only over your own NFD and its subnames, so any name you store is always interpreted as something inside your NFD's subtree. Names that reference anything outside that subtree are re-rooted under your NFD (see "The trailing-dot trap" and "Cross-root references" below).

> The examples in the tables below assume your NFD is `patrick.algo`. Substitute your own NFD wherever you see `patrick.algo` / `patrick.algo.xyz`.

**Recommended (canonical) forms:**

| You write | It serves |
|-----------|-----------|
| `@` | Your NFD itself (e.g., `patrick.algo.xyz`) |
| `www.@` | A subdomain (e.g., `www.patrick.algo.xyz`) |
| `_test._tcp.@` | An underscore-prefixed service record (SRV, etc.) |

The `@` is the standard DNS shorthand for "this zone's origin". Use the `.@` suffix for any subname, including SRV-style names like `_<service>._<proto>.@`.

**Other accepted forms** (all resolve under your NFD `patrick.algo`):

| You write | It serves |
|-----------|-----------|
| `www` | `www.patrick.algo.xyz` (bare label, no dot) |
| `www.patrick.algo` | `www.patrick.algo.xyz` (FQDN, missing trailing dot) |
| `www.patrick.algo.` | `www.patrick.algo.xyz` (canonical FQDN) |
| `www.patrick.algo.xyz` | `www.patrick.algo.xyz` (legacy mirror form, `.xyz` stripped) |
| `www.patrick.dotalgo.io` | `www.patrick.algo.xyz` (alt mirror form) |

**The trailing-dot trap (and how it's handled):**

In standard DNS zone files, a trailing dot means "fully qualified — don't append the origin". In NFD context, that's almost always a typo: your NFD has no authority outside its own subtree, so a name like `_test._tcp.` (rooted at the DNS root) couldn't serve anything as written.

To avoid this footgun, any trailing-dot name that **isn't** inside your NFD's zone (i.e., doesn't end in `.<your-nfd>.`) is treated as a relative subname and re-rooted under your NFD. So `_test._tcp.` and `_test._tcp` and `_test._tcp.@` all serve as `_test._tcp.patrick.algo.xyz`.

**Cross-root references are also re-rooted:**

The same scope rule applies even to fully-qualified names that reference *other* NFDs. If your NFD is `patrick.algo` and you store a record with name `evil.someone-else.algo.`, you do not somehow get DNS authority over `someone-else.algo` — that name gets re-rooted under your NFD, serving as `evil.someone-else.algo.patrick.algo.xyz`. The same applies to the `.algo.xyz` and `.dotalgo.io` mirror forms when they reference a different NFD root.

> **Tip:** Use the `.@` suffix form. It makes your intent explicit and works regardless of which NFD or segment the records end up loaded from.

### TTL (Time to Live)

- Minimum: 60 seconds
- Maximum: 86,400 seconds (24 hours)
- Default: 300 seconds (5 minutes)

Lower TTL = faster updates, but more DNS queries. Higher TTL = better caching, but slower propagation of changes.

---

## Supported Record Types

### A Record - IPv4 Address

Point your domain to a server's IPv4 address.

```json
{
  "name": "@",
  "type": "A",
  "rrData": ["192.168.1.1"],
  "ttl": 300
}
```

**Multiple IP addresses** (for load balancing):
```json
{
  "name": "@",
  "type": "A",
  "rrData": ["192.168.1.1", "192.168.1.2"],
  "ttl": 300
}
```

### AAAA Record - IPv6 Address

Point your domain to a server's IPv6 address.

```json
{
  "name": "@",
  "type": "AAAA",
  "rrData": ["2001:db8::1"],
  "ttl": 600
}
```

### CNAME Record - Alias

Point a subdomain to another domain name.

```json
{
  "name": "www.@",
  "type": "CNAME",
  "rrData": ["myapp.vercel.app."],
  "ttl": 300
}
```

**Common uses:**
- Point `www` to your hosting provider
- Point subdomains to cloud services (Vercel, Netlify, etc.)

### MX Record - Email

Configure where email should be delivered.

```json
{
  "name": "@",
  "type": "MX",
  "rrData": [
    "10 mail.example.com.",
    "20 backup-mail.example.com."
  ],
  "ttl": 3600
}
```

The number before the server is the **priority** - lower numbers are tried first.

### TXT Record - Text Data

Store text data for verification, email authentication, and more.

**SPF (email sender verification):**
```json
{
  "name": "@",
  "type": "TXT",
  "rrData": ["\"v=spf1 include:_spf.google.com ~all\""],
  "ttl": 300
}
```

**DMARC (email policy):**
```json
{
  "name": "_dmarc.@",
  "type": "TXT",
  "rrData": ["\"v=DMARC1; p=quarantine; rua=mailto:admin@example.com\""],
  "ttl": 3600
}
```

**Domain verification:**
```json
{
  "name": "@",
  "type": "TXT",
  "rrData": ["google-site-verification=abc123xyz"],
  "ttl": 300
}
```

### SRV Record - Service Discovery

Define the location of specific services.

```json
{
  "name": "_http._tcp.@",
  "type": "SRV",
  "rrData": ["10 5 80 web.example.com."],
  "ttl": 300
}
```

Format: `priority weight port target`
- **priority**: Lower = preferred
- **weight**: Load balancing between same priority servers
- **port**: Service port number
- **target**: Server hostname

### CAA Record - SSL Certificate Control

Specify which Certificate Authorities can issue SSL certificates for your domain.

```json
{
  "name": "@",
  "type": "CAA",
  "rrData": [
    "0 issue \"letsencrypt.org\"",
    "0 issuewild \"letsencrypt.org\""
  ],
  "ttl": 3600
}
```

**Tags:**
- `issue` - Authorize CA for regular certificates
- `issuewild` - Authorize CA for wildcard certificates
- `iodef` - Report violations to an email/URL

---

## Complete Examples

### Basic Website

Point your NFD to a web server and create a www alias:

```json
[
  {
    "name": "@",
    "type": "A",
    "rrData": ["203.0.113.50"],
    "ttl": 300
  },
  {
    "name": "www.@",
    "type": "CNAME",
    "rrData": ["@"],
    "ttl": 300
  }
]
```

### Website + Email (Google Workspace)

Host a website and receive email via Google Workspace:

```json
[
  {
    "name": "@",
    "type": "A",
    "rrData": ["203.0.113.50"],
    "ttl": 300
  },
  {
    "name": "www.@",
    "type": "CNAME",
    "rrData": ["@"],
    "ttl": 300
  },
  {
    "name": "@",
    "type": "MX",
    "rrData": [
      "1 aspmx.l.google.com.",
      "5 alt1.aspmx.l.google.com.",
      "5 alt2.aspmx.l.google.com."
    ],
    "ttl": 3600
  },
  {
    "name": "@",
    "type": "TXT",
    "rrData": ["\"v=spf1 include:_spf.google.com ~all\""],
    "ttl": 300
  }
]
```

### Vercel/Netlify Deployment

Point your domain to a cloud hosting platform:

```json
[
  {
    "name": "@",
    "type": "A",
    "rrData": ["76.76.21.21"],
    "ttl": 300
  },
  {
    "name": "www.@",
    "type": "CNAME",
    "rrData": ["cname.vercel-dns.com."],
    "ttl": 300
  }
]
```

### Full Professional Setup

Complete configuration with website, email, SSL, and verification:

```json
[
  {
    "name": "@",
    "type": "A",
    "rrData": ["203.0.113.50"],
    "ttl": 300
  },
  {
    "name": "www.@",
    "type": "CNAME",
    "rrData": ["@"],
    "ttl": 300
  },
  {
    "name": "@",
    "type": "MX",
    "rrData": [
      "10 mail.protonmail.ch.",
      "20 mailsec.protonmail.ch."
    ],
    "ttl": 3600
  },
  {
    "name": "@",
    "type": "TXT",
    "rrData": [
      "\"v=spf1 include:_spf.protonmail.ch ~all\""
    ],
    "ttl": 300
  },
  {
    "name": "_dmarc.@",
    "type": "TXT",
    "rrData": ["\"v=DMARC1; p=quarantine\""],
    "ttl": 3600
  },
  {
    "name": "@",
    "type": "CAA",
    "rrData": [
      "0 issue \"letsencrypt.org\"",
      "0 issuewild \"letsencrypt.org\""
    ],
    "ttl": 3600
  }
]
```

---

## Bluesky Integration

If you've verified your Bluesky account with your NFD, a TXT record for `_atproto` is **automatically added**. You don't need to configure this manually.

The system creates:
```
_atproto.patrick.algo.xyz. TXT "did=did:plc:abc123..."
```

This enables your NFD to serve as your Bluesky handle.

---

## Decentralized Websites (IPFS via DNSLink)

You can point your NFD at content stored on IPFS — the same idea as ENS's on-chain `contenthash`, but done with a standard DNS record, so it works with existing IPFS gateways and needs no special support.

Add a TXT record at `_dnslink.@` whose value is a DNSLink path:

- `dnslink=/ipfs/<CID>` — an immutable snapshot (the content for that exact CID)
- `dnslink=/ipns/<name>` — a mutable pointer you can update without changing DNS

```json
{
  "name": "_dnslink.@",
  "type": "TXT",
  "rrData": ["\"dnslink=/ipfs/bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi\""],
  "ttl": 300
}
```

This serves:

```
_dnslink.patrick.algo.xyz. TXT "dnslink=/ipfs/bafybei..."
```

**Viewing it:** any IPFS gateway resolves it, e.g. `https://ipfs.io/ipns/patrick.algo.xyz/` or `https://dweb.link/ipns/patrick.algo.xyz/`. For direct browser access at `patrick.algo.xyz`, point your `@` record (A/CNAME) at a DNSLink-aware gateway (e.g. a self-hosted Kubo DNSLink gateway or Cloudflare's web3 gateway); the gateway reads the `_dnslink` record by Host header and serves the IPFS content.

**Why this works:** unlike ENS — where `contenthash` lives on-chain and a gateway/browser must read the chain to translate it — the NFD DNS service answers DNS directly, so the standard DNSLink TXT convention is all you need. The `_dnslink` label is handled like any other underscore-prefixed name (the same mechanism behind `_dmarc` and `_atproto`).

---

## NFD Segments (Subdomains)

A segment (e.g. `relay.belt.algo`) is its own NFD with its own owner, and it **always serves its own DNS records** — regardless of who owns the root NFD. How a segment combines with the root depends on ownership:

**Same owner as the root NFD** — the segment is merged with the root. The root can define sub-records that fall inside the segment, and root records win if both define the same name + type. This lets you manage a root NFD and the segments you own as one zone.

**Different owner from the root NFD** — the segment alone is authoritative for its own subtree (`relay.belt.algo` and everything under it); the root NFD owner has no DNS authority inside it, and any root records pointing into the segment's subtree are ignored. This mirrors how segments are sold and operated independently.

> **Note:** "Authoritative" here describes the *NFD-ownership* boundary, not a DNS delegation. The plugin still answers these names directly — there is no NS referral and no delegated subzone (NFD subdomains never have NS records; see [Limitations](#limitations) below).

**In both cases:**
- A query for the segment resolves to the segment's own records — e.g. `relay.belt.algo` returns the A record stored on the `relay.belt.algo` NFD, even though `belt.algo` has no `relay` record.
- If the segment exists but defines no DNS records, it falls back to the default placeholder, just like a root NFD.
- Maximum depth: one record label beyond the segment (e.g. `key.relay.belt.algo`); deeper names are rejected.

---

## Limitations

1. **Segment depth**: At most one record label beyond a segment (e.g., `key.segment.patrick.algo` resolves; `a.key.segment.patrick.algo` is rejected). Leading `_`-prefixed service labels (e.g. `_test._tcp`) don't count toward this limit.

2. **No NS records for subdomains**: Your NFD subdomains are not delegated zones. NS records only work at the zone apex (`algo.xyz` itself).

3. **Record types**: The following types are supported: A, AAAA, CNAME, MX, TXT, SRV, CAA, NS, SOA, CERT

4. **Expiration**: If your NFD registration expires, DNS records will return a default placeholder until renewed.

---

## Testing Your DNS Records

After configuring your NFD, verify your records work using the `dig` command:

**Test A record:**
```bash
dig patrick.algo.xyz A
```

**Test MX records:**
```bash
dig patrick.algo.xyz MX
```

**Test TXT records:**
```bash
dig patrick.algo.xyz TXT
```

**Test a subdomain:**
```bash
dig www.patrick.algo.xyz CNAME
```

**Test Bluesky verification:**
```bash
dig _atproto.patrick.algo.xyz TXT
```

**Test IPFS DNSLink:**
```bash
dig _dnslink.patrick.algo.xyz TXT
```

You should see your configured records in the ANSWER SECTION of the response.

---

## Troubleshooting

**Records not showing up?**
- Wait a few minutes - there's caching at multiple levels
- Verify your JSON syntax is valid
- Check that record names use `@` or `subdomain.@` format

**Getting NXDOMAIN?**
- Ensure your NFD exists and is not expired
- Verify you're querying `*.algo.xyz`

**Email not working?**
- MX records must have the priority number before the hostname
- Hostnames must end with a trailing dot (e.g., `mail.example.com.`)
- Add SPF/DKIM TXT records as required by your email provider

---

## Quick Reference

| I want to... | Record Type | Example rrData |
|--------------|-------------|----------------|
| Point domain to IP | A | `["1.2.3.4"]` |
| Point to IPv6 | AAAA | `["2001:db8::1"]` |
| Create subdomain alias | CNAME | `["target.com."]` |
| Receive email | MX | `["10 mail.provider.com."]` |
| Add verification | TXT | `["verification-code"]` |
| Restrict SSL issuers | CAA | `["0 issue \"letsencrypt.org\""]` |
| Point domain to IPFS content | TXT (`_dnslink.@`) | `["\"dnslink=/ipfs/<CID>\""]` |
