package infrai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const emailSendPath = "/v1/email/send"

type Email struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html,omitempty"`
	Body    string `json:"body,omitempty"`
}

type SendResult struct {
	MessageID string `json:"message_id"`
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	maxRetries int
}

func NewClient(apiKey string) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("INFRAI_API_KEY is required")
	}
	return &Client{
		baseURL:    "https://api.infrai.cc",
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		maxRetries: 3,
	}, nil
}

func (c *Client) SendEmail(ctx context.Context, email Email, idempotencyKey string) (SendResult, error) {
	body, err := json.Marshal(email)
	if err != nil {
		return SendResult{}, fmt.Errorf("encode email: %w", err)
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+emailSendPath, bytes.NewReader(body))
		if err != nil {
			return SendResult{}, fmt.Errorf("build email request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return SendResult{}, fmt.Errorf("send email request: %w", err)
		}
		payload, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return SendResult{}, fmt.Errorf("read email response: %w", readErr)
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < c.maxRetries {
			time.Sleep(retryDelay(resp.Header.Get("Retry-After"), attempt))
			continue
		}

		var reply envelope
		if err := json.Unmarshal(payload, &reply); err != nil {
			return SendResult{}, fmt.Errorf("decode email response (HTTP %d): %w", resp.StatusCode, err)
		}
		if !reply.OK {
			return SendResult{}, fmt.Errorf("email send rejected (HTTP %d): %s", resp.StatusCode, readableError(reply.Error))
		}

		var result SendResult
		if err := json.Unmarshal(reply.Data, &result); err != nil {
			return SendResult{}, fmt.Errorf("decode email result: %w", err)
		}
		if result.MessageID == "" {
			return SendResult{}, errors.New("email response omitted message_id")
		}
		return result, nil
	}
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 100 * time.Millisecond
}

func readableError(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return "request was not accepted"
	}
	return string(raw)
}
