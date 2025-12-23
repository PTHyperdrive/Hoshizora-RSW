package main

import (
	"net/http"
	"strings"
)

// AuthMiddleware validates API tokens
func AuthMiddleware(tokens []string, next http.Handler) http.Handler {
	tokenSet := make(map[string]struct{}, len(tokens))
	for _, t := range tokens {
		tokenSet[t] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health check
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		// No tokens configured = open access (dev mode)
		if len(tokenSet) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		// Check Authorization header
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, `{"error":"missing authorization"}`, http.StatusUnauthorized)
			return
		}

		// Extract token from "Bearer <token>"
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			http.Error(w, `{"error":"invalid authorization format"}`, http.StatusUnauthorized)
			return
		}

		token := parts[1]
		if _, ok := tokenSet[token]; !ok {
			http.Error(w, `{"error":"invalid token"}`, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
