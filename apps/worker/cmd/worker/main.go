package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/lib/pq"
)

type AssignmentEvent struct {
	ID         string
	TaskID     string
	AssigneeID string
	AssignedBy string
}

type WebhookEvent struct {
	ID         string
	WebhookID  string
	WebhookURL string
	EventType  string
	Payload    []byte
	Attempts   int
}

func main() {
	dbURL := envOrDefault("DATABASE_URL", "postgresql://meridian:meridian@localhost:5432/meridian")
	intervalSec, _ := strconv.Atoi(envOrDefault("WORKER_POLL_INTERVAL_SECONDS", "5"))

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		logFatal("worker_startup", "failed to connect to database", map[string]any{"error": err.Error()})
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		logFatal("worker_startup", "database ping failed", map[string]any{"error": err.Error()})
	}

	logInfo("worker_started", "Meridian worker started", map[string]any{
		"poll_interval_seconds": intervalSec,
	})

	ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
	defer ticker.Stop()

	for {
		if err := processAssignmentEvents(db); err != nil {
			logError("assignment_processing", err.Error(), nil)
		}
		if err := processWebhookEvents(db); err != nil {
			logError("webhook_processing", err.Error(), nil)
		}
		<-ticker.C
	}
}

func processAssignmentEvents(db *sql.DB) error {
	events, err := fetchUnprocessedAssignmentEvents(db)
	if err != nil {
		return err
	}

	for _, event := range events {
		if err := handleAssignment(db, event); err != nil {
			logError("assignment_event", err.Error(), map[string]any{"event_id": event.ID})
			continue
		}
		if err := markAssignmentProcessed(db, event.ID); err != nil {
			return err
		}
		logInfo("assignment_processed", "processed assignment event", map[string]any{
			"event_id": event.ID,
			"task_id":  event.TaskID,
		})
	}
	return nil
}

func fetchUnprocessedAssignmentEvents(db *sql.DB) ([]AssignmentEvent, error) {
	rows, err := db.Query(`
		SELECT id, task_id, assignee_id, assigned_by
		FROM task_assignment_events
		WHERE processed = FALSE
		ORDER BY created_at ASC
		LIMIT 10
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []AssignmentEvent
	for rows.Next() {
		var e AssignmentEvent
		if err := rows.Scan(&e.ID, &e.TaskID, &e.AssigneeID, &e.AssignedBy); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func handleAssignment(db *sql.DB, event AssignmentEvent) error {
	if isViewerOnTaskTeam(db, event.AssigneeID, event.TaskID) {
		logInfo("assignment_skipped", "skipping notification for viewer role", map[string]any{
			"assignee_id": event.AssigneeID,
			"task_id":     event.TaskID,
		})
		return nil
	}

	var taskTitle string
	err := db.QueryRow("SELECT title FROM tasks WHERE id = $1", event.TaskID).Scan(&taskTitle)
	if err != nil {
		return fmt.Errorf("fetch task title: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO notifications (user_id, type, title, body, metadata)
		VALUES ($1, 'task_assigned', $2, $3, $4)
	`,
		event.AssigneeID,
		fmt.Sprintf("Task assigned: %s", taskTitle),
		"You have been assigned a new task.",
		fmt.Sprintf(`{"task_id": "%s", "assigned_by": "%s"}`, event.TaskID, event.AssignedBy),
	)
	return err
}

func isViewerOnTaskTeam(db *sql.DB, userID, taskID string) bool {
	// QA: skip notifications for viewer-role assignees
	var role string
	err := db.QueryRow(`
		SELECT tm.role
		FROM tasks t
		JOIN projects p ON p.id = t.project_id
		JOIN team_members tm ON tm.team_id = p.team_id
		WHERE t.id = $1 AND tm.user_id = $2
	`, taskID, userID).Scan(&role)
	if err != nil {
		return false
	}
	return role == "viewer"
}

func markAssignmentProcessed(db *sql.DB, eventID string) error {
	_, err := db.Exec("UPDATE task_assignment_events SET processed = TRUE WHERE id = $1", eventID)
	return err
}

func processWebhookEvents(db *sql.DB) error {
	events, err := fetchUnprocessedWebhookEvents(db)
	if err != nil {
		return err
	}

	for _, event := range events {
		if err := dispatchWebhook(event); err != nil {
			recordWebhookFailure(db, event, err)
			continue
		}
		if err := markWebhookProcessed(db, event.ID); err != nil {
			return err
		}
		logInfo("webhook_dispatched", "webhook event delivered", map[string]any{
			"event_id":   event.ID,
			"event_type": event.EventType,
			"url":        event.WebhookURL,
		})
	}
	return nil
}

func fetchUnprocessedWebhookEvents(db *sql.DB) ([]WebhookEvent, error) {
	rows, err := db.Query(`
		SELECT we.id, we.webhook_id, w.url, we.event_type, we.payload::text, we.attempts
		FROM webhook_events we
		JOIN webhooks w ON w.id = we.webhook_id
		WHERE we.processed = FALSE AND we.attempts < 3
		ORDER BY we.created_at ASC
		LIMIT 10
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []WebhookEvent
	for rows.Next() {
		var e WebhookEvent
		if err := rows.Scan(&e.ID, &e.WebhookID, &e.WebhookURL, &e.EventType, &e.Payload, &e.Attempts); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func dispatchWebhook(event WebhookEvent) error {
	req, err := http.NewRequest(http.MethodPost, event.WebhookURL, bytes.NewReader(event.Payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Meridian-Event", event.EventType)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func recordWebhookFailure(db *sql.DB, event WebhookEvent, err error) {
	_, dbErr := db.Exec(`
		UPDATE webhook_events
		SET attempts = attempts + 1, last_error = $1
		WHERE id = $2
	`, err.Error(), event.ID)
	if dbErr != nil {
		logError("webhook_failure_record", dbErr.Error(), map[string]any{"event_id": event.ID})
	}
	logError("webhook_dispatch", err.Error(), map[string]any{
		"event_id": event.ID,
		"url":      event.WebhookURL,
	})
}

func markWebhookProcessed(db *sql.DB, eventID string) error {
	_, err := db.Exec("UPDATE webhook_events SET processed = TRUE WHERE id = $1", eventID)
	return err
}

func logInfo(event, message string, fields map[string]any) {
	logStructured("INFO", event, message, fields)
}

func logError(event, message string, fields map[string]any) {
	logStructured("ERROR", event, message, fields)
}

func logFatal(event, message string, fields map[string]any) {
	logStructured("FATAL", event, message, fields)
	os.Exit(1)
}

func logStructured(level, event, message string, fields map[string]any) {
	entry := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"level":     level,
		"service":   "meridian-worker",
		"event":     event,
		"message":   message,
	}
	for k, v := range fields {
		entry[k] = v
	}
	data, _ := json.Marshal(entry)
	log.Print(string(data))
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}