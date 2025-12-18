# Ollama Integration for Drift Detection

The drift detector uses Ollama for generating semantic embeddings of log templates.

## Setup

### 1. Install Ollama

```bash
curl -fsSL https://ollama.com/install.sh | sh
```

### 2. Pull Embedding Model

```bash
ollama pull embeddinggemma
```

### 3. Verify Ollama is Running

Test the embedding API:

```bash
curl -X POST http://localhost:11434/api/embed \
  -H "Content-Type: application/json" \
  -d '{
    "model": "embeddinggemma",
    "input": [
      "Test log message 1",
      "Test log message 2"
    ]
  }'
```

Expected response:
```json
{
  "embeddings": [
    [0.123, 0.456, ...],
    [0.789, 0.012, ...]
  ],
  "model": "embeddinggemma"
}
```

## Configuration

Add to `datadog.yaml`:

```yaml
logs_config:
  drift_detection:
    enabled: true
    embedding_url: "http://localhost:11434/api/embed"
    embedding_model: "embeddinggemma"
```

## API Details

### Request Format

```json
POST /api/embed
Content-Type: application/json

{
  "model": "embeddinggemma",
  "input": ["text1", "text2", ...]
}
```

### Response Format

```json
{
  "embeddings": [
    [0.1, 0.2, ...],  // 768-dimensional vector
    [0.3, 0.4, ...]   // 768-dimensional vector
  ],
  "model": "embeddinggemma"
}
```

## Batching

The drift detector automatically batches templates for efficiency:
- **Max batch size**: 100 templates per request
- **Batch timeout**: 5 seconds
- **Retry logic**: 3 attempts with exponential backoff

## Performance

### Ollama Default Settings
- Ollama runs on CPU by default
- For better performance, Ollama will use GPU if available
- Expected latency: 50-200ms per batch (depending on hardware)

### Scaling Considerations

For high-volume deployments:
1. **GPU acceleration**: Ensure Ollama can use GPU
2. **Multiple instances**: Run multiple Ollama instances behind a load balancer
3. **Batch size tuning**: Adjust `batch_timeout` in config for better batching

## Troubleshooting

### Ollama Not Running

```bash
# Check if Ollama is running
curl http://localhost:11434/api/tags

# Start Ollama if not running (usually runs as systemd service)
systemctl start ollama
```

### Model Not Available

```bash
# List available models
ollama list

# Pull the model if missing
ollama pull embeddinggemma
```

### Connection Errors

Check the agent logs for:
```
Failed to get embeddings after retries: http request: connection refused
```

Solution:
1. Verify Ollama is running: `systemctl status ollama`
2. Check the URL in config matches Ollama's endpoint
3. Ensure no firewall blocking port 11434

### High Latency

If embedding requests are slow:
1. Check Ollama resource usage: `ollama ps`
2. Consider GPU acceleration if available
3. Increase `batch_timeout` to accumulate more templates per request
4. Reduce number of active drift detectors via `max_idle_time` config

## Alternative Models

You can use other embedding models supported by Ollama:

```bash
# List available embedding models
ollama list | grep embed

# Try alternative models
ollama pull nomic-embed-text
ollama pull mxbai-embed-large
```

Update `embedding_model` in config accordingly:
```yaml
logs_config:
  drift_detection:
    embedding_model: "nomic-embed-text"  # Alternative model
```

**Note**: Ensure the model outputs 768-dimensional embeddings, or adjust the DMD analyzer accordingly.

## Monitoring

The drift detector logs embedding performance:

```
INFO | Embedding batch processed: 45 templates in 123ms
WARN | Embedding request failed (attempt 1/3): timeout
```

Monitor Prometheus metrics:
- `logdrift_embedding_requests_total` - Total embedding requests
- `logdrift_embedding_errors_total` - Failed embedding requests
- `logdrift_embedding_latency_seconds` - Embedding request latency
