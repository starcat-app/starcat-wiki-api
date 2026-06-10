package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/dong4j/starcat-wiki-api/internal/handler"
	"github.com/dong4j/starcat-wiki-api/internal/middleware"
	"github.com/dong4j/starcat-wiki-api/internal/probe"
	"github.com/dong4j/starcat-wiki-api/internal/scheduler"
	"github.com/dong4j/starcat-wiki-api/internal/store"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("[env] no .env file found, using OS environment only")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "5004"
	}

	storeFile := os.Getenv("STORE_FILE")
	if storeFile == "" {
		storeFile = "./wiki.db"
	}

	apiKeysStr := os.Getenv("API_KEYS")
	if apiKeysStr == "" {
		log.Fatal("API_KEYS env is required")
	}
	apiKeys := strings.Split(apiKeysStr, ",")

	ua := os.Getenv("PROBE_USER_AGENT")
	enableRPC := os.Getenv("ENABLE_CODEWIKI_BATCHEXECUTE") == "true"

	// SQLite Store
	sqliteStore, err := store.NewSQLiteStore(storeFile)
	if err != nil {
		log.Fatalf("Failed to initialize SQLite: %v", err)
	}
	defer sqliteStore.Close()

	// Probes
	baseReq := probe.NewBaseRequest(ua)
	probes := probe.DefaultRegistry(baseReq, enableRPC)

	// Scheduler
	sch := scheduler.New(sqliteStore)

	// Handler
	probeHandler := handler.NewProbeHandler(sqliteStore, probes)

	// Bearer Auth
	authMW := middleware.NewBearerAuth(apiKeys)

	// Router
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthzHandler)
	mux.Handle("GET /api/v1/wikis", authMW.Wrap(http.HandlerFunc(probeHandler.HandleProbeV1)))
	mux.Handle("POST /api/v1/wikis/batch", authMW.Wrap(http.HandlerFunc(probeHandler.HandleProbeBatchV1)))
	mux.Handle("POST /internal/sync/probe", authMW.Wrap(handler.HandleAdminSyncProbe(sch)))
	mux.Handle("POST /internal/refresh/owner", authMW.Wrap(handler.HandleAdminRefreshOwner(sch)))

	// Graceful Shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("Received %v, shutting down...", sig)
		sch.Stop()
		sqliteStore.Close()
		os.Exit(0)
	}()

	go sch.Start()

	log.Printf("starcat-wiki-api starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
