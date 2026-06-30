// Package main starcat-wiki-api 入口。
//
// v2 变更：
//   - 启动时恢复 probing 中的任务
//   - 注册错误重试定时任务
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

	// Handler
	probeHandler := handler.NewProbeHandler(sqliteStore, probes)

	// Scheduler（注入 retry 回调）
	sch := scheduler.New(sqliteStore, probeHandler.RetryErrors)

	// Bearer Auth
	authMW := middleware.NewBearerAuth(apiKeys)

	// Router
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthzHandler)
	// R-03 (2026-06-11): /api/v1/ping 专门给 Starcat 客户端「测试连接」按钮用，
	// 在 middleware 后面挂——同时验证服务可达 + Bearer Key 正确。详见 handler/ping.go。
	mux.Handle("GET /api/v1/ping", authMW.Wrap(handler.HandlePingV1("wiki")))
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

	// 启动定时任务
	go sch.Start()

	// v2: 启动恢复 probing 中的任务
	go probeHandler.RecoverPendingProbes()

	log.Printf("starcat-wiki-api starting on port %s", port)
	log.Printf("Endpoints:")
	log.Printf("  GET  /api/v1/ping                   - Connectivity probe for Starcat client")
	log.Printf("  GET  /api/v1/wikis?owner=X&repo=Y  - Single probe")
	log.Printf("  POST /api/v1/wikis/batch             - Batch probe (max 50, async)")
	log.Printf("  POST /internal/sync/probe             - Manual sync trigger")
	log.Printf("  POST /internal/refresh/owner          - Owner refresh")
	log.Printf("  GET  /healthz                         - Health check")
	handler := middleware.CORS(mux)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
