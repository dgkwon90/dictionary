package syncpush

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"neulsang/desktopd/internal/domain/outbox"
)

const defaultTimeout = 10 * time.Second

type Client struct {
	baseURL    string
	httpClient *http.Client
}

var _ outbox.Publisher = (*Client)(nil)

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

func (c *Client) Publish(ctx context.Context, events []outbox.Event) (resultErr error) {
	body, err := json.Marshal(publishRequest{Events: toPublishEvents(events)})
	if err != nil {
		return fmt.Errorf("marshal sync events: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build sync request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("post sync events: %w", err)
	}
	defer func() {
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("drain sync response body: %w", err))
		}
		if err := resp.Body.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close sync response body: %w", err))
		}
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("post sync events: status %s", resp.Status)
	}
	return nil
}

func toPublishEvents(events []outbox.Event) []publishEvent {
	out := make([]publishEvent, 0, len(events))
	for _, event := range events {
		out = append(out, publishEvent{
			EventID:       event.EventID,
			AggregateType: event.AggregateType,
			AggregateID:   event.AggregateID,
			EventType:     event.EventType,
			PayloadJSON:   event.PayloadJSON,
			CreatedAt:     event.CreatedAt.UTC(),
		})
	}
	return out
}

type publishRequest struct {
	Events []publishEvent `json:"events"`
}

type publishEvent struct {
	EventID       string    `json:"event_id"`
	AggregateType string    `json:"aggregate_type"`
	AggregateID   string    `json:"aggregate_id"`
	EventType     string    `json:"event_type"`
	PayloadJSON   string    `json:"payload_json"`
	CreatedAt     time.Time `json:"created_at"`
}
