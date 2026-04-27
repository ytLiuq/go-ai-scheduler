package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// Payload is sent to the webhook when a task exhausts all retries.
type Payload struct {
	Type               string    `json:"type"`
	TaskID             int64     `json:"task_id"`
	TaskName           string    `json:"task_name"`
	InstanceID         int64     `json:"instance_id"`
	ScheduleInstanceID string    `json:"schedule_instance_id"`
	RetryCount         int       `json:"retry_count"`
	MaxRetry           int       `json:"max_retry"`
	ErrorCode          string    `json:"error_code"`
	ErrorMessage       string    `json:"error_message"`
	Timestamp          time.Time `json:"timestamp"`
}

// Alerter posts alert payloads to a webhook URL.
type Alerter struct {
	webhookURL string
	client     *http.Client
	logger     *log.Logger
}

// New creates an Alerter. If webhookURL is empty, alerts are logged to stdout.
func New(webhookURL string, logger *log.Logger) *Alerter {
	return &Alerter{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 5 * time.Second},
		logger:     logger,
	}
}

// Send delivers one alert.
func (a *Alerter) Send(ctx context.Context, p Payload) {
	p.Timestamp = time.Now()
	p.Type = "task_failure"

	if a.webhookURL == "" {
		data, _ := json.Marshal(p)
		a.logger.Printf("ALERT %s", string(data))
		return
	}

	body, err := json.Marshal(p)
	if err != nil {
		a.logger.Printf("alert marshal error: %v", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.webhookURL, bytes.NewReader(body))
	if err != nil {
		a.logger.Printf("alert build request error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		a.logger.Printf("alert send error: %v", err)
		return
	}
	resp.Body.Close()

	if resp.StatusCode >= 300 {
		a.logger.Printf("alert webhook returned status %d", resp.StatusCode)
	}
}
