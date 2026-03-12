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

- `@` - Your domain itself (e.g., `patrick.algo.xyz`)
- `www.@` - A subdomain (becomes `www.patrick.algo.xyz`)
- `mail.@` - Another subdomain (becomes `mail.patrick.algo.xyz`)

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

## NFD Segments (Subdomains)

If you own NFD segments (subdomains like `mail.patrick.algo`), their DNS records are automatically merged with the root NFD.

**Rules:**
- Segment must be owned by the same account as the root NFD
- Root NFD records take priority if there's a conflict
- Maximum depth: 3 labels (e.g., `a.b.patrick.algo`)

---

## Limitations

1. **Segment depth**: Maximum of 3 labels after your NFD name (e.g., `a.b.c.patrick.algo` is the limit)

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
