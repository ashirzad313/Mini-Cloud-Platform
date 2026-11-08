package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

func main() {
	mux := http.NewServeMux()

	// Liveness: the HTTP server is running, even if PostgreSQL is unavailable.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Connectivity: connect to PostgreSQL through the cluster's 'postgres' Service.
	// pgx reads PGHOST, PGPORT, PGUSER, PGDATABASE, and PGPASSWORD from the environment.
	mux.HandleFunc("GET /db-health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		conn, err := pgx.Connect(ctx, "")
		if err != nil {
			log.Printf("connect to PostgreSQL: %v", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		defer conn.Close(context.Background())

		var database string
		if err := conn.QueryRow(ctx, "SELECT current_database()").Scan(&database); err != nil {
			log.Printf("query PostgreSQL: %v", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status":   "connected",
			"database": database,
		})
	})

	log.Println("Management API listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func writeJSON(w http.ResponseWriter, status int, body map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("write JSON response: %v", err)
	}
}
