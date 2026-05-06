package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/pkg/errors"
)

const groqEndpoint = "https://api.groq.com/openai/v1/chat/completions"

// GroqClient is a thin client for Groq's OpenAI-compatible API.
// We avoid an SDK dependency to keep the surface small and explicit.
type GroqClient struct {
	apiKey     string
	model      string
	timeout    time.Duration
	maxRetries int
	http       *http.Client
}

func NewGroqClient(apiKey, model string, timeout time.Duration, maxRetries int) *GroqClient {
	return &GroqClient{
		apiKey:     apiKey,
		model:      model,
		timeout:    timeout,
		maxRetries: maxRetries,
		http:       &http.Client{Timeout: timeout},
	}
}

// --- Request types ---

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []Tool    `json:"tools,omitempty"`
	ToolChoice  string    `json:"tool_choice,omitempty"` // "auto" | "none" | "required"
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// --- Response types ---

type ChatResponse struct {
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
	Model   string   `json:"model"`
}

type Choice struct {
	Message      ResponseMessage `json:"message"`
	FinishReason string          `json:"finish_reason"`
}

type ResponseMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	ToolCalls []RespToolCall `json:"tool_calls,omitempty"`
}

type RespToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolCallFunc `json:"function"`
}

type ToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // raw JSON string per OpenAI spec
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// --- Rate limit handling ---

// RateLimitError is returned when Groq responds with HTTP 429.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limited (retry after %s)", e.RetryAfter.Round(time.Millisecond))
}

// retryAfterRe matches Groq's "Please try again in 6.76s" / "60ms" phrasing.
var retryAfterRe = regexp.MustCompile(`try again in ([\d.]+)(ms|s)`)

func parseRetryAfter(body []byte) time.Duration {
	m := retryAfterRe.FindSubmatch(body)
	if m == nil {
		return 10 * time.Second
	}
	val, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		return 10 * time.Second
	}
	if string(m[2]) == "ms" {
		return time.Duration(val * float64(time.Millisecond))
	}
	return time.Duration(val * float64(time.Second))
}

// Chat sends a request and retries automatically on 429 rate-limit responses,
// waiting exactly as long as Groq instructs plus a small 200ms buffer.
func (c *GroqClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, time.Duration, []byte, error) {
	for attempt := 0; ; attempt++ {
		resp, elapsed, body, err := c.doChat(ctx, req)
		if err == nil {
			return resp, elapsed, body, nil
		}
		rle, isRateLimit := err.(*RateLimitError)
		if !isRateLimit || attempt >= c.maxRetries {
			return nil, elapsed, body, err
		}
		wait := rle.RetryAfter + 200*time.Millisecond
		slog.Info("rate limited; retrying", "attempt", attempt+1, "wait", wait.Round(time.Millisecond))
		select {
		case <-ctx.Done():
			return nil, elapsed, body, ctx.Err()
		case <-time.After(wait):
		}
	}
}

// doChat performs a single HTTP request without retry logic.
func (c *GroqClient) doChat(ctx context.Context, req ChatRequest) (*ChatResponse, time.Duration, []byte, error) {
	if req.Model == "" {
		req.Model = c.model
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, 0, nil, errors.Wrap(err, "marshal request")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, groqEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, nil, errors.Wrap(err, "build http request")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	start := time.Now()
	resp, err := c.http.Do(httpReq)
	elapsed := time.Since(start)
	if err != nil {
		return nil, elapsed, nil, errors.Wrap(err, "http do")
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, elapsed, nil, errors.Wrap(err, "read body")
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, elapsed, respBody, &RateLimitError{RetryAfter: parseRetryAfter(respBody)}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, elapsed, respBody, errors.Errorf("groq api error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, elapsed, respBody, errors.Wrap(err, "unmarshal response")
	}

	return &chatResp, elapsed, respBody, nil
}

// ParseToolCallArgs decodes the JSON-string arguments field into a generic map.
func ParseToolCallArgs(raw string) (map[string]interface{}, error) {			
	if raw == "" {
		return map[string]interface{}{}, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("parse tool call args: %w", err)
	}
	return m, nil
}
