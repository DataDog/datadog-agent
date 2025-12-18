# Embedding Service API Specification

## Overview

The embedding service provides vector embeddings for log templates. The service is accessed via HTTP POST requests.

## Endpoint

```
POST /embed
Content-Type: application/json
```

## Request Format

```json
{
  "texts": [
    "2025-12-10 <*> UTC | CORE | INFO | Server started on port <*>",
    "2025-12-10 <*> UTC | CORE | WARN | Connection timeout after <*>"
  ],
  "model": "embeddinggemma",
  "batch_size": 100,
  "normalize": true
}
```

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `texts` | string[] | Yes | Array of log templates to embed |
| `model` | string | Yes | Model name (e.g., "embeddinggemma") |
| `batch_size` | int | No | Max batch size (default: 100) |
| `normalize` | bool | No | L2 normalize vectors (default: true) |

### Constraints

- `texts`: Max 100 items per request
- `texts[i]`: Max 8000 characters per text
- `model`: Must be supported by server

## Response Format

### Success (200 OK)

```json
{
  "embeddings": [
    [0.1952, -0.0159, 0.0321, ..., -0.0234],
    [-0.0423, 0.0871, -0.0234, ..., 0.0456]
  ],
  "dimension": 768,
  "processing_time_ms": 1234,
  "model_version": "v1.0.0",
  "status": "success"
}
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `embeddings` | float[][] | 2D array: [n_texts, dimension] |
| `dimension` | int | Embedding vector size (768) |
| `processing_time_ms` | int | Server-side processing time |
| `model_version` | string | Model version used |
| `status` | string | "success" or "error" |

## Error Responses

### 400 Bad Request

```json
{
  "error": {
    "code": "INVALID_REQUEST",
    "message": "Field 'texts' is required",
    "details": {
      "field": "texts",
      "constraint": "required"
    }
  },
  "status": "error"
}
```

### 429 Too Many Requests

```json
{
  "error": {
    "code": "RATE_LIMIT_EXCEEDED",
    "message": "Rate limit exceeded: 100 requests/minute",
    "retry_after_ms": 5000
  },
  "status": "error"
}
```

### 503 Service Unavailable

```json
{
  "error": {
    "code": "SERVICE_UNAVAILABLE",
    "message": "Embedding model is loading",
    "retry_after_ms": 30000
  },
  "status": "error"
}
```

## Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `INVALID_REQUEST` | 400 | Malformed request body |
| `BATCH_TOO_LARGE` | 400 | texts[] exceeds max batch size |
| `TEXT_TOO_LONG` | 400 | Individual text exceeds max length |
| `MODEL_NOT_FOUND` | 404 | Requested model doesn't exist |
| `RATE_LIMIT_EXCEEDED` | 429 | Too many requests |
| `SERVICE_UNAVAILABLE` | 503 | Service temporarily unavailable |
| `INTERNAL_ERROR` | 500 | Unexpected server error |

## Go Client Example

```go
package main

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

type EmbedRequest struct {
    Texts     []string `json:"texts"`
    Model     string   `json:"model"`
    BatchSize int      `json:"batch_size,omitempty"`
    Normalize bool     `json:"normalize"`
}

type EmbedResponse struct {
    Embeddings     [][]float64 `json:"embeddings"`
    Dimension      int         `json:"dimension"`
    ProcessingTime int         `json:"processing_time_ms"`
    ModelVersion   string      `json:"model_version"`
    Status         string      `json:"status"`
}

type EmbeddingClient struct {
    baseURL    string
    httpClient *http.Client
    model      string
}

func NewEmbeddingClient(baseURL, model string) *EmbeddingClient {
    return &EmbeddingClient{
        baseURL: baseURL,
        httpClient: &http.Client{
            Timeout: 30 * time.Second,
            Transport: &http.Transport{
                MaxIdleConns:        100,
                MaxIdleConnsPerHost: 10,
                IdleConnTimeout:     90 * time.Second,
            },
        },
        model: model,
    }
}

func (c *EmbeddingClient) Embed(ctx context.Context, texts []string) ([][]float64, error) {
    req := &EmbedRequest{
        Texts:     texts,
        Model:     c.model,
        BatchSize: 100,
        Normalize: true,
    }

    body, err := json.Marshal(req)
    if err != nil {
        return nil, fmt.Errorf("marshal request: %w", err)
    }

    httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/embed", bytes.NewReader(body))
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }
    httpReq.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("http request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("http error: %d", resp.StatusCode)
    }

    var embedResp EmbedResponse
    if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
        return nil, fmt.Errorf("decode response: %w", err)
    }

    return embedResp.Embeddings, nil
}
```

## Performance Expectations

- **Latency**: <100ms for batch of 100 texts
- **Throughput**: 1000 texts/second
- **Concurrency**: Up to 10 concurrent requests per client
