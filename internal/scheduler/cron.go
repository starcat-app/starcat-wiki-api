package scheduler

import (
	"log"
	"sync"

	"github.com/robfig/cron/v3"
	"github.com/dong4j/starcat-wiki-api/internal/store"
)

type Scheduler struct {
	store   store.Store
	cron    *cron.Cron
	mu      sync.Mutex
	running map[string]bool
}

func New(s store.Store) *Scheduler {
	sch := &Scheduler{
		store:   s,
		cron:    cron.New(cron.WithSeconds()),
		running: make(map[string]bool),
	}

	// 03:00 扫过期
	sch.cron.AddFunc("0 0 3 * * *", sch.refreshStale)
	// 04:00 清理
	sch.cron.AddFunc("0 0 4 * * *", sch.cleanupExpired)
	// 周日 05:00 健康检查
	sch.cron.AddFunc("0 0 5 * * 0", sch.healthCheck)

	return sch
}

func (sch *Scheduler) Start() {
	sch.cron.Start()
	log.Println("[scheduler] cron started")
}

func (sch *Scheduler) Stop() {
	ctx := sch.cron.Stop()
	<-ctx.Done()
	log.Println("[scheduler] stopped")
}

func (sch *Scheduler) refreshStale() {
	if !sch.tryLock("refresh") {
		return
	}
	defer sch.unlock("refresh")
	log.Println("[scheduler] running refreshStale (placeholder)")
}

func (sch *Scheduler) cleanupExpired() {
	if !sch.tryLock("cleanup") {
		return
	}
	defer sch.unlock("cleanup")
	log.Println("[scheduler] running cleanupExpired (placeholder)")
}

func (sch *Scheduler) healthCheck() {
	if !sch.tryLock("healthCheck") {
		return
	}
	defer sch.unlock("healthCheck")
	log.Println("[scheduler] running healthCheck (placeholder)")
}

func (sch *Scheduler) tryLock(name string) bool {
	sch.mu.Lock()
	defer sch.mu.Unlock()
	if sch.running[name] {
		return false
	}
	sch.running[name] = true
	return true
}

func (sch *Scheduler) unlock(name string) {
	sch.mu.Lock()
	sch.running[name] = false
	sch.mu.Unlock()
}
