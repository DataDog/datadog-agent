// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ollama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type Client struct {
	endpoint string
	model    string
}

func NewClient(endpoint, model string) *Client {
	return &Client{
		endpoint: endpoint,
		model:    model,
	}
}

type Chat struct {
	client   *Client
	system   string
	messages []Message
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string                 `json:"model"`
	Messages []Message              `json:"messages"`
	Options  map[string]interface{} `json:"options,omitempty"`
	Format   interface{}            `json:"format,omitempty"`
	Stream   bool                   `json:"stream"`
}

type ChatResponse struct {
	Model   string  `json:"model"`
	Message Message `json:"message"`
	Done    bool    `json:"done"`
}

type GenerateOption func(*generateConfig)

type generateConfig struct {
	temperature *float64
	format      interface{}
	debug       bool
	trace       bool
}

func WithTemperature(t float64) GenerateOption {
	return func(c *generateConfig) {
		c.temperature = &t
	}
}

func WithFormat(f interface{}) GenerateOption {
	return func(c *generateConfig) {
		c.format = f
	}
}

func WithDebug() GenerateOption {
	return func(c *generateConfig) {
		c.debug = true
	}
}

func WithTrace() GenerateOption {
	return func(c *generateConfig) {
		c.trace = true
	}
}

func (c *Client) CreateChat(systemPrompt string) *Chat {
	messages := []Message{}
	if systemPrompt != "" {
		messages = append(messages, Message{Role: "system", Content: systemPrompt})
	}
	return &Chat{
		client:   c,
		system:   systemPrompt,
		messages: messages,
	}
}

func (ch *Chat) Generate(userMessage string, opts ...GenerateOption) (*ChatResponse, error) {
	cfg := &generateConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	ch.messages = append(ch.messages, Message{Role: "user", Content: userMessage})

	req := ChatRequest{
		Model:    ch.client.model,
		Messages: ch.messages,
		Stream:   false,
	}

	if cfg.temperature != nil {
		if req.Options == nil {
			req.Options = make(map[string]interface{})
		}
		req.Options["temperature"] = *cfg.temperature
	}

	if cfg.format != nil {
		req.Format = cfg.format
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if cfg.debug {
		fmt.Fprintf(os.Stderr, "Prompt: %s\n", userMessage)
	}
	body, err := ch.postWithRetry("/api/chat", jsonData, 3, 100*time.Millisecond)
	if err != nil {
		ch.messages = ch.messages[:len(ch.messages)-1]
		return nil, err
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		ch.messages = ch.messages[:len(ch.messages)-1]
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var respErr error
	if chatResp.Message.Role == "" {
		respErr = fmt.Errorf("invalid response: message role is empty")
	}
	if !chatResp.Done {
		respErr = fmt.Errorf("invalid response: message not done")
	}

	if respErr != nil {
		ch.messages = ch.messages[:len(ch.messages)-1]
		return nil, respErr
	}
	ch.messages = append(ch.messages, chatResp.Message)

	if cfg.debug {
		fmt.Fprintf(os.Stderr, "Response: %s\n", chatResp.Message.Content)
	}
	if cfg.trace {
		fmt.Fprintf(os.Stderr, "****************************************\n")
		for _, message := range ch.messages {
			fmt.Fprintf(os.Stderr, "--------------------------------\n")
			fmt.Fprintf(os.Stderr, "%s: %s\n", message.Role, message.Content)
		}
		fmt.Fprintf(os.Stderr, "****************************************\n")
	}
	return &chatResp, nil
}

func (ch *Chat) postWithRetry(path string, data []byte, maxRetries int, backoff time.Duration) ([]byte, error) {
	url := ch.client.endpoint + path
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
		if err != nil {
			lastErr = fmt.Errorf("failed to connect to Ollama: %w", err)
			if i < maxRetries-1 {
				time.Sleep(backoff * time.Duration(i+1))
				continue
			}
			return nil, lastErr
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return body, nil
		}

		lastErr = fmt.Errorf("Ollama API returned status %d: %s", resp.StatusCode, string(body))
		if i < maxRetries-1 {
			time.Sleep(backoff * time.Duration(i+1))
			continue
		}
	}

	return nil, lastErr
}
