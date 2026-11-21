// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fx

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type shutdownRoundTripper func(*http.Request) (*http.Response, error)

func (f shutdownRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func shutdownTestSpans() []*logging.Span {
	return []*logging.Span{{Service: "dd-agent-fx-init", Name: "stopSystemProbe", Resource: "system-probe", TraceID: 123, SpanID: 456, Duration: 1234, Meta: map[string]string{"agent.version": "test"}}}
}

func TestShutdownDirectIntakePayload(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("api_key", "key-before-rotation", model.SourceAgentRuntime)
	cfg.Set("env", "shutdown-test", model.SourceAgentRuntime)
	cfg.Set("cluster_name", "mathewe-shutdown", model.SourceAgentRuntime)
	cfg.Set("tags", []string{"test:shutdown"}, model.SourceAgentRuntime)
	cfg.Set("apm_config.apm_dd_url", "https://intake.example", model.SourceAgentRuntime)
	sender := newShutdownSender(cfg, "cached-host")
	// A synchronized/rotated key available at shutdown must replace the startup value.
	cfg.Set("api_key", "resolved-current-key", model.SourceAgentRuntime)
	calls := 0
	sender.client.Transport = shutdownRoundTripper(func(req *http.Request) (*http.Response, error) {
		calls++
		assert.Equal(t, "https://intake.example/api/v0.2/traces", req.URL.String())
		assert.Equal(t, "resolved-current-key", req.Header.Get("DD-API-KEY"))
		assert.Equal(t, "application/x-protobuf", req.Header.Get("Content-Type"))
		require.Equal(t, "gzip", req.Header.Get("Content-Encoding"))
		zr, err := gzip.NewReader(req.Body)
		require.NoError(t, err)
		data, err := io.ReadAll(zr)
		require.NoError(t, err)
		require.NoError(t, zr.Close())
		var payload pb.AgentPayload
		require.NoError(t, payload.UnmarshalVT(data))
		assert.Equal(t, "cached-host", payload.HostName)
		assert.Equal(t, "shutdown-test", payload.Env)
		require.Len(t, payload.TracerPayloads, 1)
		require.Len(t, payload.TracerPayloads[0].Chunks, 1)
		chunk := payload.TracerPayloads[0].Chunks[0]
		assert.Equal(t, int32(2), chunk.Priority)
		require.Len(t, chunk.Spans, 1)
		span := chunk.Spans[0]
		assert.Equal(t, uint64(123), span.TraceID)
		assert.Equal(t, "mathewe-shutdown", span.Meta["kube_cluster_name"])
		assert.Equal(t, float64(1), span.Metrics["_dd.analyzed"])
		return &http.Response{StatusCode: 202, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	require.NoError(t, sender.export(context.Background(), shutdownTestSpans()))
	assert.Equal(t, 1, calls)
	assert.ErrorIs(t, sender.client.CheckRedirect(nil, nil), http.ErrUseLastResponse)
}

func TestShutdownExportBoundedAfterTraceAgentExit(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("api_key", "test-key", model.SourceAgentRuntime)
	synctest.Test(t, func(t *testing.T) {
		sender := newShutdownSender(cfg, "host")
		calls := 0
		sender.client.Transport = shutdownRoundTripper(func(req *http.Request) (*http.Response, error) {
			calls++
			<-req.Context().Done()
			return nil, req.Context().Err()
		})
		before := time.Now()
		sender.send(shutdownTestSpans())
		assert.Equal(t, shutdownExportTimeout, time.Since(before))
		assert.Equal(t, 1, calls, "no retry loop may extend process termination")
	})
}

func TestShutdownDestinationPolicyAndCredentials(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  string
		main bool
	}{
		{"no-key", "", true}, {"unresolved-secret", "ENC[private]", true},
		{"unresolved-delegated-auth", "DELA[11111111-1111-1111-1111-111111111111, aws]", true}, {"primary-disabled", "private-key", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.Set("api_key", tc.key, model.SourceAgentRuntime)
			cfg.Set("apm_config.traces_send_to_main_endpoint", tc.main, model.SourceAgentRuntime)
			sender := newShutdownSender(cfg, "host")
			sender.client.Transport = shutdownRoundTripper(func(*http.Request) (*http.Response, error) {
				t.Error("unexpected network request")
				return nil, errors.New("private error")
			})
			err := sender.export(context.Background(), shutdownTestSpans())
			require.Error(t, err)
			if tc.key != "" {
				assert.NotContains(t, err.Error(), tc.key)
			}
		})
	}
}

func TestShutdownDoesNotExposeTransportSecrets(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("api_key", "private-api-key", model.SourceAgentRuntime)
	sender := newShutdownSender(cfg, "host")
	sender.client.Transport = shutdownRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("https://user:password@host?api_key=private-api-key")
	})
	err := sender.export(context.Background(), shutdownTestSpans())
	require.Error(t, err)
	assert.EqualError(t, err, "trace intake request failed")
}

func TestShutdownAdditionalEndpoints(t *testing.T) {
	for _, primary := range []bool{true, false} {
		t.Run(map[bool]string{true: "staging-dual-shipping", false: "additional-only"}[primary], func(t *testing.T) {
			cfg := configmock.New(t)
			key := ""
			if primary {
				key = "primary-key"
			}
			cfg.Set("api_key", key, model.SourceAgentRuntime)
			cfg.Set("apm_config.apm_dd_url", "https://primary.example", model.SourceAgentRuntime)
			cfg.Set("apm_config.traces_send_to_main_endpoint", primary, model.SourceAgentRuntime)
			cfg.Set("apm_config.additional_endpoints", map[string][]string{"https://secondary.example": {"secondary-key", "second-org-key"}}, model.SourceAgentRuntime)
			type request struct {
				url, key string
				body     []byte
			}
			requests := make(chan request, 3)
			sender := newShutdownSender(cfg, "host")
			sender.client.Transport = shutdownRoundTripper(func(req *http.Request) (*http.Response, error) {
				data, err := io.ReadAll(req.Body)
				if err != nil {
					return nil, err
				}
				requests <- request{req.URL.String(), req.Header.Get("DD-API-KEY"), data}
				return &http.Response{StatusCode: 202, Body: io.NopCloser(strings.NewReader(""))}, nil
			})
			require.NoError(t, sender.export(context.Background(), shutdownTestSpans()))
			close(requests)
			var got []string
			var payload []byte
			for req := range requests {
				got = append(got, req.url+"|"+req.key)
				if payload == nil {
					payload = req.body
				} else {
					assert.Equal(t, payload, req.body)
				}
			}
			expected := []string{"https://secondary.example/api/v0.2/traces|secondary-key", "https://secondary.example/api/v0.2/traces|second-org-key"}
			if primary {
				expected = append(expected, "https://primary.example/api/v0.2/traces|primary-key")
			}
			assert.ElementsMatch(t, expected, got)
		})
	}
}

func TestShutdownSlowPrimaryDoesNotBlockSecondary(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("api_key", "primary-key", model.SourceAgentRuntime)
	cfg.Set("apm_config.apm_dd_url", "https://slow.example", model.SourceAgentRuntime)
	cfg.Set("apm_config.additional_endpoints", map[string][]string{"https://fast.example": {"secondary-key"}}, model.SourceAgentRuntime)
	synctest.Test(t, func(t *testing.T) {
		secondary := make(chan struct{}, 1)
		sender := newShutdownSender(cfg, "host")
		sender.client.Transport = shutdownRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "slow.example" {
				<-req.Context().Done()
				return nil, req.Context().Err()
			}
			secondary <- struct{}{}
			return &http.Response{StatusCode: 202, Body: io.NopCloser(strings.NewReader(""))}, nil
		})
		before := time.Now()
		sender.send(shutdownTestSpans())
		assert.Equal(t, shutdownExportTimeout, time.Since(before))
		assert.Len(t, secondary, 1, "secondary must be attempted within the same budget")
	})
}

func TestShutdownUnresolvedPrimaryStillSendsSecondary(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("api_key", "ENC[private-primary]", model.SourceAgentRuntime)
	cfg.Set("apm_config.additional_endpoints", map[string][]string{"https://secondary.example": {"resolved-secondary"}}, model.SourceAgentRuntime)
	sender := newShutdownSender(cfg, "host")
	sent := make(chan string, 1)
	sender.client.Transport = shutdownRoundTripper(func(req *http.Request) (*http.Response, error) {
		sent <- req.Header.Get("DD-API-KEY")
		return &http.Response{StatusCode: 202, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	err := sender.export(context.Background(), shutdownTestSpans())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "private-primary")
	require.Len(t, sent, 1)
	assert.Equal(t, "resolved-secondary", <-sent)
}
