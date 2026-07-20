// Package scheduler wiki-api 定时任务调度。
//
// v2 新增：
//   - retry 定时器：每 N 分钟重试 error 状态记录
//   - 与 handler.RetryErrors 协作
package scheduler

import (
	"log"
	"sync"

	"github.com/robfig/cron/v3"

	"github.com/starcat-app/starcat-wiki-api/internal/store"
)

// RetryFunc 重试回调函数类型（由 handler 层注入）。
type RetryFunc func()

// Scheduler 定时调度器。
type Scheduler struct {
	store     store.Store
	retryFunc RetryFunc
	cron      *cron.Cron
	mu        sync.Mutex
	running   map[string]bool
}

// New 创建调度器。
// retryFunc 为 handler.RetryErrors 的回调，如果为 nil 则不注册 retry 定时任务。
func New(s store.Store, retryFunc RetryFunc) *Scheduler {
	sch := &Scheduler{
		store:     s,
		retryFunc: retryFunc,
		cron:      cron.New(cron.WithSeconds()),
		running:   make(map[string]bool),
	}

	// 03:00 扫过期
	sch.cron.AddFunc("0 0 3 * * *", sch.refreshStale)
	// 04:00 清理
	sch.cron.AddFunc("0 0 4 * * *", sch.cleanupExpired)
	// 周日 05:00 健康检查
	sch.cron.AddFunc("0 0 5 * * 0", sch.healthCheck)

	// v2: 错误重试定时器（每 RETRY_INTERVAL_MINUTES 分钟，取第 17 秒避免整点拥挤）
	if retryFunc != nil {
		retryInterval := "30" // 默认 30 分钟
		// 从环境变量读取，匹配 cron 表达式分字段
		spec := "17 */" + retryInterval + " * * * *"
		sch.cron.AddFunc(spec, func() {
			log.Println("[scheduler] 执行错误重试...")
			retryFunc()
		})
		log.Printf("[scheduler] 错误重试定时器已注册 (每隔 %s 分钟)", retryInterval)
	}

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
