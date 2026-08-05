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

// maxResponseBodyBytes bounds how much of the sync endpoint's response we drain.
// The response body is discarded either way; this just caps the effort spent
// reading it (review R-01/R-08, RW-02).
const maxResponseBodyBytes = 1 << 20 // 1MiB

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
		if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBodyBytes)); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("drain sync response body: %w", err))
		}
		if err := resp.Body.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close sync response body: %w", err))
		}
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if permanentStatus(resp.StatusCode) {
			return fmt.Errorf("post sync events: %w: status %s", outbox.ErrPermanent, resp.Status)
		}
		return fmt.Errorf("post sync events: status %s", resp.Status)
	}
	return nil
}

// permanentStatus reports whether the server is saying "never" rather than "not now".
//
// A 4xx means the request itself is the problem, so sending it again unchanged will
// get the same answer forever — and because the outbox drains oldest-first, that one
// event would block every event behind it. Two exceptions are explicitly about timing
// rather than content: 408 (the server gave up waiting) and 429 (slow down). 5xx is
// the server's own problem and is always worth retrying.
func permanentStatus(code int) bool {
	switch code {
	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return false
	default:
		return code >= 400 && code < 500
	}
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
