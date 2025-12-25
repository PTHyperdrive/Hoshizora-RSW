package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	cfg := defaultConfig()

	// Parse flags
	flag.IntVar(&cfg.Port, "port", cfg.Port, "HTTPS server port")
	flag.StringVar(&cfg.DBPath, "db", cfg.DBPath, "SQLite database path")
	flag.StringVar(&cfg.MasterKey, "master-key", "", "Master key for encrypting stored keys (required)")
	flag.StringVar(&cfg.CertFile, "cert", cfg.CertFile, "TLS certificate file")
	flag.StringVar(&cfg.KeyFile, "key", cfg.KeyFile, "TLS private key file")

	var authTokensFlag string
	flag.StringVar(&authTokensFlag, "tokens", "", "Comma-separated API tokens (empty = no auth)")

	var httpMode bool
	flag.BoolVar(&httpMode, "http", false, "Use HTTP instead of HTTPS (dev only)")

	flag.Parse()

	// Environment variable overrides
	if envMaster := os.Getenv("KEYSAVER_MASTER_KEY"); envMaster != "" {
		cfg.MasterKey = envMaster
	}
	if envTokens := os.Getenv("KEYSAVER_TOKENS"); envTokens != "" {
		authTokensFlag = envTokens
	}

	// Validate master key
	if cfg.MasterKey == "" {
		log.Fatal("Master key is required. Use --master-key or KEYSAVER_MASTER_KEY env var")
	}

	// Parse auth tokens
	if authTokensFlag != "" {
		cfg.AuthTokens = strings.Split(authTokensFlag, ",")
		for i := range cfg.AuthTokens {
			cfg.AuthTokens[i] = strings.TrimSpace(cfg.AuthTokens[i])
		}
		log.Printf("[auth] %d API tokens configured", len(cfg.AuthTokens))
	} else {
		log.Printf("[auth] WARNING: No API tokens configured, running in open mode")
	}

	// Initialize storage
	storage, err := NewStorage(cfg.DBPath, cfg.MasterKey)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer storage.Close()
	log.Printf("[storage] initialized at %s", cfg.DBPath)

	// Create server
	srv := NewServer(storage, cfg)
	handler := srv.Handler()

	// HTTP server configuration
	httpSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	if httpMode {
		// Development mode: plain HTTP
		log.Printf("[server] starting HTTP server on :%d (DEV MODE)", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	} else {
		// Production mode: HTTPS with TLS
		// Check if cert files exist
		if _, err := os.Stat(cfg.CertFile); os.IsNotExist(err) {
			log.Printf("[tls] Certificate file not found: %s", cfg.CertFile)
			log.Printf("[tls] To generate a self-signed cert for testing:")
			log.Printf("      openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 365 -nodes -subj '/CN=localhost'")
			log.Fatal("[tls] Cannot start HTTPS server without certificates")
		}

		// TLS configuration with modern security settings
		// Include AES_128_GCM ciphers required for HTTP/2
		tlsConfig := &tls.Config{
			MinVersion:               tls.VersionTLS12,
			PreferServerCipherSuites: true,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			},
		}
		httpSrv.TLSConfig = tlsConfig

		log.Printf("[server] starting HTTPS server on :%d", cfg.Port)
		if err := httpSrv.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile); err != nil {
			log.Fatalf("HTTPS server error: %v", err)
		}
	}
}
