package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/starcat-app/starcat-wiki-api/internal/scheduler"
)

func HandleAdminSyncProbe(sch *scheduler.Scheduler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := fmt.Sprintf("task-%s-probe-%d", time.Now().Format("2006-01-02T15:04:05Z"), time.Now().UnixNano()%1000)

		writeJSON(w, map[string]string{
			"task_id":    taskID,
			"started_at": time.Now().Format(time.RFC3339),
			"status":     "running",
		})
	}
}

func HandleAdminRefreshOwner(sch *scheduler.Scheduler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := fmt.Sprintf("task-%s-owner-%d", time.Now().Format("2006-01-02T15:04:05Z"), time.Now().UnixNano()%1000)

		writeJSON(w, map[string]string{
			"task_id":    taskID,
			"started_at": time.Now().Format(time.RFC3339),
			"status":     "running",
		})
	}
}
