// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package logsagentpipelineimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	compressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type stubHostname struct{}

func (stubHostname) Get(context.Context) (string, error) { return "test-host", nil }
func (stubHostname) GetWithProvider(context.Context) (hostnameinterface.Data, error) {
	return hostnameinterface.Data{Hostname: "test-host", Provider: "test"}, nil
}
func (stubHostname) GetSafe(context.Context) string { return "test-host" }

// recordingIntake answers log payloads with the status returned by statusFor, which receives the
// messages of the payload, and records the messages of the payloads it accepted.
type recordingIntake struct {
	*httptest.Server
	statusFor func(messages []string) int

	mu        sync.Mutex
	payloads  int
	delivered []string
}

func newRecordingIntake(t *testing.T, statusFor func(messages []string) int) *recordingIntake {
	t.Helper()
	intake := &recordingIntake{statusFor: statusFor}
	intake.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var records []struct {
			Message string `json:"message"`
		}
		require.NoError(t, json.NewDecoder(bytes.NewReader(body)).Decode(&records))
		messages := make([]string, len(records))
		for i, r := range records {
			messages[i] = r.Message
		}
		status := intake.statusFor(messages)
		intake.mu.Lock()
		intake.payloads++
		if status == http.StatusOK {
			intake.delivered = append(intake.delivered, messages...)
		}
		intake.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(intake.Close)
	return intake
}

func (in *recordingIntake) stats() (payloads int, delivered []string) {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.payloads, append([]string(nil), in.delivered...)
}

func okStatus([]string) int { return http.StatusOK }

func isRetryable(err error) bool {
	var retryable *client.RetryableError
	return errors.As(err, &retryable)
}

func newTestSyncSender(t *testing.T, intakeURL string, overrides map[string]interface{}) *SyncSender {
	t.Helper()
	u, err := url.Parse(intakeURL)
	require.NoError(t, err)
	settings := map[string]interface{}{
		"api_key":                 "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"logs_config.logs_dd_url": u.Host,
		"logs_config.logs_no_ssl": true,
	}
	for k, v := range overrides {
		settings[k] = v
	}
	s, err := NewSyncSender(Dependencies{
		Log:          logmock.New(t),
		Config:       configComponent.NewMockWithOverrides(t, settings),
		Hostname:     stubHostname{},
		Compression:  compressionfx.NewMockCompressor(),
		IntakeOrigin: config.DDOTIntakeOrigin,
	})
	require.NoError(t, err)
	return s
}

func testMessages(contents ...string) []*message.Message {
	source := sources.NewLogSource("otel", &config.LogsConfig{})
	msgs := make([]*message.Message, len(contents))
	for i, content := range contents {
		msgs[i] = message.NewMessage([]byte(content), message.NewOrigin(source), message.StatusInfo, 0)
	}
	return msgs
}

func TestSyncSenderDelivers(t *testing.T) {
	intake := newRecordingIntake(t, okStatus)
	s := newTestSyncSender(t, intake.URL, nil)

	errs := s.Send(context.Background(), testMessages("a", "b", "c"))

	assert.Equal(t, []error{nil, nil, nil}, errs)
	payloads, delivered := intake.stats()
	assert.Equal(t, 1, payloads)
	assert.Equal(t, []string{"a", "b", "c"}, delivered)
}

func TestSyncSenderSplitsPayloadsAtTheBatchSize(t *testing.T) {
	intake := newRecordingIntake(t, okStatus)
	s := newTestSyncSender(t, intake.URL, map[string]interface{}{"logs_config.batch_max_size": 2})

	errs := s.Send(context.Background(), testMessages("a", "b", "c", "d", "e"))

	assert.Equal(t, make([]error, 5), errs)
	payloads, delivered := intake.stats()
	assert.Equal(t, 3, payloads)
	assert.ElementsMatch(t, []string{"a", "b", "c", "d", "e"}, delivered)
}

func TestSyncSenderReportsIntakeErrors(t *testing.T) {
	tests := []struct {
		status    int
		retryable bool
	}{
		{status: http.StatusForbidden},
		{status: http.StatusRequestEntityTooLarge},
		{status: http.StatusServiceUnavailable, retryable: true},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			intake := newRecordingIntake(t, func([]string) int { return tt.status })
			s := newTestSyncSender(t, intake.URL, nil)

			errs := s.Send(context.Background(), testMessages("a", "b"))

			require.Len(t, errs, 2)
			for _, err := range errs {
				require.Error(t, err)
				assert.Equal(t, tt.retryable, isRetryable(err))
			}
		})
	}
}

func TestSyncSenderReportsErrorsPerPayload(t *testing.T) {
	intake := newRecordingIntake(t, func(messages []string) int {
		for _, m := range messages {
			if strings.HasPrefix(m, "fail") {
				return http.StatusServiceUnavailable
			}
		}
		return http.StatusOK
	})
	s := newTestSyncSender(t, intake.URL, map[string]interface{}{"logs_config.batch_max_size": 2})

	errs := s.Send(context.Background(), testMessages("a", "b", "fail c", "d"))

	assert.NoError(t, errs[0])
	assert.NoError(t, errs[1])
	assert.True(t, isRetryable(errs[2]), "fail c: %v", errs[2])
	assert.True(t, isRetryable(errs[3]), "d shares the payload of fail c: %v", errs[3])
	_, delivered := intake.stats()
	assert.Equal(t, []string{"a", "b"}, delivered)
}

func TestSyncSenderRejectsMessagesLargerThanAPayload(t *testing.T) {
	intake := newRecordingIntake(t, okStatus)
	s := newTestSyncSender(t, intake.URL, map[string]interface{}{"logs_config.batch_max_content_size": 1000})

	errs := s.Send(context.Background(), testMessages("a", strings.Repeat("x", 2000), "b"))

	assert.NoError(t, errs[0])
	require.Error(t, errs[1])
	assert.False(t, isRetryable(errs[1]))
	assert.NoError(t, errs[2])
	payloads, delivered := intake.stats()
	assert.Equal(t, 1, payloads, "a message that fits no payload must not split the others")
	assert.Equal(t, []string{"a", "b"}, delivered)
}

func TestSyncSenderAppliesProcessingRules(t *testing.T) {
	intake := newRecordingIntake(t, okStatus)
	s := newTestSyncSender(t, intake.URL, map[string]interface{}{
		"logs_config.processing_rules": `[{"type": "exclude_at_match", "name": "drop", "pattern": "drop me"}]`,
	})

	errs := s.Send(context.Background(), testMessages("keep me", "drop me"))

	assert.Equal(t, []error{nil, nil}, errs, "a message filtered out by a processing rule is not an error")
	_, delivered := intake.stats()
	assert.Equal(t, []string{"keep me"}, delivered)
}

func TestSyncSenderSendsToEveryReliableEndpoint(t *testing.T) {
	main := newRecordingIntake(t, okStatus)
	additional := newRecordingIntake(t, func([]string) int { return http.StatusForbidden })
	additionalURL, err := url.Parse(additional.URL)
	require.NoError(t, err)
	s := newTestSyncSender(t, main.URL, map[string]interface{}{
		"logs_config.additional_endpoints": `[{"api_key": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "host": "` + additionalURL.Hostname() +
			`", "port": ` + additionalURL.Port() + `, "use_ssl": false}]`,
	})

	errs := s.Send(context.Background(), testMessages("a"))

	require.Error(t, errs[0], "a failure on an additional reliable endpoint must be reported")
	assert.False(t, isRetryable(errs[0]))
	_, delivered := main.stats()
	assert.Equal(t, []string{"a"}, delivered)
}

func TestAgentIsASyncSenderFactory(t *testing.T) {
	intake := newRecordingIntake(t, okStatus)
	u, err := url.Parse(intake.URL)
	require.NoError(t, err)
	agent := NewLogsAgent(Dependencies{
		Log: logmock.New(t),
		Config: configComponent.NewMockWithOverrides(t, map[string]interface{}{
			"api_key":                 "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"logs_enabled":            true,
			"logs_config.logs_dd_url": u.Host,
			"logs_config.logs_no_ssl": true,
		}),
		Hostname:     stubHostname{},
		Compression:  compressionfx.NewMockCompressor(),
		IntakeOrigin: config.DDOTIntakeOrigin,
	})
	require.NotNil(t, agent)

	s, err := agent.(*Agent).NewSyncSender()

	require.NoError(t, err)
	assert.Equal(t, []error{nil}, s.Send(context.Background(), testMessages("a")))
}
