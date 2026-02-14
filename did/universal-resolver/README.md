# Universal Resolver Driver for did:nfd

This directory contains configuration for integrating the `did:nfd` resolver as a driver for the [DIF Universal Resolver](https://github.com/decentralized-identity/universal-resolver).

## Overview

The `did:nfd` driver resolves Decentralized Identifiers for Algorand Non-Fungible Domains (NFDs). It queries the Algorand blockchain to construct W3C DID Documents from NFD on-chain properties.

| Property | Value |
|----------|-------|
| DID Method | `nfd` |
| DID Pattern | `^did:nfd:([a-z0-9]{1,27}\.){1,2}algo$` |
| Blockchain | Algorand |
| Key Type | Ed25519 |
| Driver Port | 8080 (internal) |

## Files

| File | Description |
|------|-------------|
| `docker-compose.yml` | Docker Compose service definition for the driver |
| `driver.json` | Universal Resolver driver configuration |
| `README.md` | This file |

## Standalone Deployment

Build and run the driver independently:

```bash
# Build the Docker image
docker buildx build -f did/Dockerfile -t did-nfd-resolver:latest .

# Run the container
docker run -p 8080:8080 \
  -e ALGOD_URL=https://mainnet-api.4160.nodely.dev \
  -e ALGOD_TOKEN="" \
  -e REGISTRY_ID=760937186 \
  -e CACHE_TTL=5m \
  did-nfd-resolver:latest
```

Test resolution:

```bash
curl http://localhost:8080/1.0/identifiers/did:nfd:patrick.algo
```

## Universal Resolver Integration

To add this driver to an existing Universal Resolver deployment:

1. Copy `docker-compose.yml` service definition into your Universal Resolver's `docker-compose.yml`
2. Add `driver.json` to the Universal Resolver's driver configuration
3. Restart the Universal Resolver

### Adding to Universal Resolver docker-compose.yml

```yaml
services:
  # ... existing services ...

  driver-did-nfd:
    image: ghcr.io/txnlab/did-nfd-resolver:latest
    ports:
      - "8167:8080"
    environment:
      ALGOD_URL: "https://mainnet-api.4160.nodely.dev"
      ALGOD_TOKEN: ""
      REGISTRY_ID: "760937186"
      CACHE_TTL: "5m"
```

### Adding to uni-resolver-web config

Add to `application.yml` or equivalent:

```yaml
drivers:
  - pattern: "^did:nfd:([a-z0-9]{1,27}\\.){1,2}algo$"
    url: "http://driver-did-nfd:8080/1.0/identifiers/"
    testIdentifiers:
      - "did:nfd:patrick.algo"
```

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `LISTEN` | `:8080` | HTTP listen address |
| `ALGOD_URL` | `https://mainnet-api.4160.nodely.dev` | Algorand algod API URL |
| `ALGOD_TOKEN` | (empty) | Algorand algod API token |
| `REGISTRY_ID` | `760937186` | NFD Registry application ID |
| `CACHE_TTL` | `5m` | DID document cache TTL |

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/1.0/identifiers/{did}` | GET | Resolve a DID to a DID Document |
| `/health` | GET | Health check |
| `/1.0/properties` | GET | Method properties and capabilities |

### Response Codes

| Code | Meaning |
|------|---------|
| 200 | Successful resolution |
| 400 | Invalid DID format |
| 404 | NFD not found |
| 410 | DID deactivated (expired/for-sale NFD) |
| 500 | Internal error |

## Health Check

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

## Method Properties

```bash
curl http://localhost:8080/1.0/properties
```

Returns supported features and DID method metadata.
