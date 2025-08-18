package rum

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToLogs_SuccessfulConversion(t *testing.T) {
	payload := map[string]any{
		"service": map[string]any{
			"name":    "test-service",
			"version": "1.0.0",
		},
		"session": map[string]any{
			"id": "test-session-id",
		},
		"user": map[string]any{
			"id": "test-user-id",
		},
		"device": map[string]any{
			"type": "mobile",
		},
		"message": "Test log message",
		"level":   "info",
	}

	req, err := http.NewRequest("POST", "https://example.com/rum", nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	logs := ToLogs(payload, req)

	assert.NotNil(t, logs)
	assert.Equal(t, 1, logs.ResourceLogs().Len())

	resourceLogs := logs.ResourceLogs().At(0)

	resource := resourceLogs.Resource()
	assert.NotNil(t, resource)

	assert.Equal(t, 1, resourceLogs.ScopeLogs().Len())
	scopeLogs := resourceLogs.ScopeLogs().At(0)
	assert.Equal(t, InstrumentationScopeName, scopeLogs.Scope().Name())

	logRecord := scopeLogs.LogRecords().At(0)

	attributes := logRecord.Attributes()
	assert.Greater(t, attributes.Len(), 0)
}
