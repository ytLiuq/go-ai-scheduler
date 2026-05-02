package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/repo"
	"github.com/example/go-ai-scheduler/internal/rpc"
	"github.com/example/go-ai-scheduler/internal/scheduler/dispatch"
	"github.com/example/go-ai-scheduler/internal/scheduler/route"
)

// EventHandler receives external events and triggers matching tasks.
type EventHandler struct {
	tasks      repo.TaskRepository
	instances  repo.TaskInstanceRepository
	router     *route.Router
	dispatcher *dispatch.Client
	logger     *log.Logger
}

// NewEventHandler creates an EventHandler.
func NewEventHandler(
	tasks repo.TaskRepository,
	instances repo.TaskInstanceRepository,
	router *route.Router,
	dispatcher *dispatch.Client,
	l *log.Logger,
) *EventHandler {
	return &EventHandler{
		tasks:      tasks,
		instances:  instances,
		router:     router,
		dispatcher: dispatcher,
		logger:     l,
	}
}

type publishEventRequest struct {
	EventName string                 `json:"event_name"`
	Payload   map[string]interface{} `json:"payload"`
}

// Publish handles POST /api/v1/events/publish.
func (h *EventHandler) Publish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req publishEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}
	if req.EventName == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "event_name is required"})
		return
	}

	ctx := context.Background()
	triggered := 0

	// Find all enabled event-triggered tasks matching this event name.
	allTasks, err := h.tasks.ListTasks(ctx)
	if err != nil {
		h.logger.Printf("event handler: list tasks failed: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	for _, task := range allTasks {
		if task.TriggerType != "event" || task.EventName != req.EventName {
			continue
		}
		if task.Status != "enabled" {
			continue
		}

		h.logger.Printf("event %s matched task_id=%d", req.EventName, task.ID)

		shardTotal := task.TotalShards
		if shardTotal <= 0 {
			shardTotal = 1
		}
		for shard := 0; shard < shardTotal; shard++ {
			instance := &model.TaskInstance{
				TaskID:             task.ID,
				ScheduleInstanceID: generateScheduleInstanceID(task.ID),
				ShardNo:            shard,
				ShardTotal:         task.TotalShards,
				TriggerTime:        time.Now(),
				Status:             "pending",
			}
			if err := h.instances.CreateInstance(ctx, instance); err != nil {
				h.logger.Printf("event handler: create instance failed task_id=%d err=%v", task.ID, err)
				continue
			}
			worker, err := h.router.Pick(ctx, route.SelectOptions{
				Labels:   model.DecodeLabels(task.Labels),
				Strategy: task.RouteStrategy,
			})
			if err != nil {
				h.logger.Printf("event handler: pick worker failed task_id=%d err=%v", task.ID, err)
				continue
			}
			_ = h.instances.UpdateInstanceDispatch(ctx, instance.ID, worker.ID, time.Now().Format(time.RFC3339Nano))
			if err := h.dispatcher.Dispatch(ctx, worker, rpc.ExecuteTaskRequest{
				ScheduleInstanceID: instance.ScheduleInstanceID,
				TaskID:             task.ID,
				TaskType:           task.Type,
				Payload:            renderEventPayload(task.Payload, req),
				TimeoutSeconds:     task.TimeoutSeconds,
				ShardNo:            shard,
				ShardTotal:         task.TotalShards,
				IdempotencyKey:     task.IdempotencyKey,
			}); err != nil {
				h.logger.Printf("event handler: dispatch failed task_id=%d err=%v", task.ID, err)
				continue
			}
			triggered++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"event":     req.EventName,
		"triggered": triggered,
	})
}

func generateScheduleInstanceID(taskID int64) string {
	return fmt.Sprintf("task-%d-%d", taskID, time.Now().UnixNano())
}

// renderEventPayload replaces template variables in the task payload with event data.
// Supports: {{.event.name}}, {{.event.payload.key}}, {{.event.payload.nested.key}}
func renderEventPayload(template string, req publishEventRequest) string {
	result := template
	result = strings.ReplaceAll(result, "{{.event.name}}", req.EventName)
	for k, v := range req.Payload {
		placeholder := fmt.Sprintf("{{.event.payload.%s}}", k)
		strVal := fmt.Sprintf("%v", v)
		result = strings.ReplaceAll(result, placeholder, strVal)
	}
	return result
}
