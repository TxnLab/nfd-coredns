/*
 * Copyright (c) 2025. TxnLab Inc.
 * All Rights reserved.
 */

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"

	"github.com/TxnLab/nfd-coredns/did/lib/did"
)

// Config holds the server configuration.
type Config struct {
	Listen     string
	AlgodURL   string
	AlgodToken string
	RegistryID uint64
	CacheTTL   time.Duration
}

func main() {
	cfg := loadConfig()

	// Create algod client
	client, err := algod.MakeClient(cfg.AlgodURL, cfg.AlgodToken)
	if err != nil {
		log.Fatalf("Failed to create algod client: %v", err)
	}

	// Create resolver
	resolver := did.NewNfdDIDResolver(client, cfg.RegistryID, cfg.CacheTTL)

	// Set up HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("GET /1.0/identifiers/{did...}", handleResolve(resolver))
	mux.HandleFunc("GET /1.0/properties", handleProperties)
	mux.HandleFunc("GET /health", handleHealth)

	log.Printf("Starting DID resolver on %s (algod: %s, registry: %d, cache: %s)",
		cfg.Listen, cfg.AlgodURL, cfg.RegistryID, cfg.CacheTTL)

	if err := http.ListenAndServe(cfg.Listen, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func loadConfig() Config {
	cfg := Config{
		Listen:     envOrDefault("LISTEN", ":8080"),
		AlgodURL:   envOrDefault("ALGOD_URL", "https://mainnet-api.4160.nodely.dev"),
		AlgodToken: envOrDefault("ALGOD_TOKEN", ""),
		RegistryID: 760937186,
		CacheTTL:   5 * time.Minute,
	}

	if v := os.Getenv("REGISTRY_ID"); v != "" {
		if id, err := strconv.ParseUint(v, 10, 64); err == nil {
			cfg.RegistryID = id
		}
	}

	if v := os.Getenv("CACHE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.CacheTTL = d
		}
	}

	return cfg
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func handleResolve(resolver did.NfdDIDResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		didStr := r.PathValue("did")
		if didStr == "" {
			writeError(w, http.StatusBadRequest, "missing DID parameter")
			return
		}

		// Determine content type from Accept header
		contentType := did.ContentTypeDIDJSON
		accept := r.Header.Get("Accept")
		if strings.Contains(accept, did.ContentTypeDIDLDJSON) {
			contentType = did.ContentTypeDIDLDJSON
		}

		result, err := resolver.Resolve(r.Context(), didStr)
		if err != nil {
			if result != nil && result.ResolutionMetadata.Error != "" {
				switch result.ResolutionMetadata.Error {
				case did.ErrorNotFound:
					w.Header().Set("Content-Type", contentType)
					w.WriteHeader(http.StatusNotFound)
					json.NewEncoder(w).Encode(result)
					return
				case did.ErrorInvalidDID:
					w.Header().Set("Content-Type", contentType)
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(result)
					return
				}
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Check for deactivated
		statusCode := http.StatusOK
		if result.DocumentMetadata.Deactivated {
			statusCode = http.StatusGone
		}

		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(statusCode)
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(result)
	}
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

func handleProperties(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]interface{}{
		"method":  "nfd",
		"network": "algorand",
		"properties": map[string]interface{}{
			"identifierFormat":     "did:nfd:<name>.algo",
			"identifierRegex":      "^did:nfd:([a-z0-9]{1,27}\\.){1,2}algo$",
			"blockchain":           "algorand",
			"keyType":              "Ed25519",
			"supportsDeactivation": true,
			"supportsExpiration":   true,
			"supportsKeyAgreement": true,
			"supportsServices":     true,
		},
	})
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}
