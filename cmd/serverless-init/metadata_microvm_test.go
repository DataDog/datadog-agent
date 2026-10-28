// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/lifecycle"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/metadata/agentmetadata"
	serverlessdef "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/def"
	serverlessinitimpl "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/impl"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	serverlessenv "github.com/DataDog/datadog-agent/pkg/serverless/env"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type lockedMetadataSerializer struct {
	serializer.MetricSerializer
	mu       sync.Mutex
	payloads []*agentmetadata.Payload
}

func (s *lockedMetadataSerializer) SendMetadata(payload marshaler.JSONMarshaler) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payloads = append(s.payloads, payload.(*agentmetadata.Payload))
	return nil
}

func (s *lockedMetadataSerializer) sent() []*agentmetadata.Payload {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*agentmetadata.Payload{}, s.payloads...)
}

type microVMMetadataTelemetry struct{}

func (microVMMetadataTelemetry) Flush() {}
func (microVMMetadataTelemetry) AddEnhancedMetric(string, float64, metrics.MetricSource, float64, ...string) {
}

func newMicroVMMetadataComponent(t *testing.T, service *cloudservice.MicroVM, enabled bool, firstRunDelay int) (serverlessinitimpl.Provides, *lockedMetadataSerializer, *metadataLifecycle) {
	t.Helper()
	serial := &lockedMetadataSerializer{}
	lc := &metadataLifecycle{}
	p := serverlessinitimpl.NewComponent(serverlessinitimpl.Requires{
		Lc: lc,
		Config: config.NewMockWithOverrides(t, map[string]any{
			"serverless.metadata_enabled":       enabled,
			"inventories_first_run_delay":       firstRunDelay,
			"inventories_configuration_enabled": false,
			"site":                              "datadoghq.eu",
		}),
		Log:      logmock.New(t),
		Hostname: fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule()),
		Params: serverlessdef.Params{
			Tags:           map[string]string{"service": "test-service", "env": "test-env", "version": "test-version"},
			WrappedCommand: []string{"python", "app.py"},
		},
		Provider:   service,
		Serializer: serial,
	})
	return p, serial, lc
}

func TestMicroVMMetadataLifecycleReports(t *testing.T) {
	for _, forwarding := range []bool{false, true} {
		for _, delay := range []int{0, 3600} {
			t.Run(fmt.Sprintf("forwarding=%t/delay=%d", forwarding, delay), func(t *testing.T) {
				const imageARN = "arn:aws:lambda:us-east-1:123456789012:microvm-image:my-image"
				t.Setenv(serverlessenv.MicroVMImageARNEnvVar, imageARN)
				service := &cloudservice.MicroVM{}
				p, serial, lc := newMicroVMMetadataComponent(t, service, true, delay)
				require.NotNil(t, p.Provider.Callback)

				// The runner's first collection waits for the identity /run delivers.
				collectCtx, cancelCollect := context.WithCancel(context.Background())
				collected := make(chan struct{})
				go func() { p.Provider.Callback(collectCtx); close(collected) }()
				t.Cleanup(func() { cancelCollect(); <-collected })

				var forwarded atomic.Int32
				var forwarder *lifecycle.Forwarder
				if forwarding {
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						forwarded.Add(1)
						w.WriteHeader(http.StatusAccepted)
					}))
					t.Cleanup(upstream.Close)
					forwarder = lifecycle.NewForwarder(upstream.Listener.Addr().(*net.TCPAddr).Port, 5*time.Second, 5*time.Second, 5*time.Second)
				}
				telemetry := microVMMetadataTelemetry{}
				srv := lifecycle.NewServer(0, telemetry, telemetry, nil, telemetry, nil,
					metrics.MetricSourceAWSMicroVMEnhanced, time.Second, lifecycle.NewNoopChildHandle(), forwarder,
					lifecycle.HookToggles{Run: true, Resume: true}, nil)
				srv.SetMetadataReporter(service.MetadataReporter(p.Comp))
				require.NoError(t, srv.ListenAndServe(func(err error) { assert.NoError(t, err) }))
				t.Cleanup(func() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					assert.NoError(t, srv.Stop(ctx))
				})
				client := &http.Client{Timeout: 5 * time.Second}
				post := func(hook, body string, expected int) {
					t.Helper()
					url := fmt.Sprintf("http://127.0.0.1:%d/aws/lambda-microvms/runtime/v1/%s", srv.Addr().(*net.TCPAddr).Port, hook)
					resp, err := client.Post(url, "application/json", strings.NewReader(body))
					require.NoError(t, err)
					_, err = io.Copy(io.Discard, resp.Body)
					require.NoError(t, resp.Body.Close())
					require.NoError(t, err)
					status := http.StatusOK
					if hook == "ready" {
						status = http.StatusServiceUnavailable // No managed child in this fixture.
					}
					if forwarding && (hook == "run" || hook == "resume") {
						status = http.StatusAccepted
					}
					require.Equal(t, status, resp.StatusCode)
					require.Eventually(t, func() bool { return len(serial.sent()) == expected }, 5*time.Second, time.Millisecond)
				}
				// uuid labels the steps whose payloads share one UUID.
				steps := []struct {
					name, hook, body string
					payloads         int
					id, uuid         string
				}{
					{name: "ready before run", hook: "ready"},
					{name: "validate before run", hook: "validate"},
					{name: "resume before run", hook: "resume"},
					{name: "run without body", hook: "run"},
					{name: "run with empty object", hook: "run", body: "{}"},
					{name: "run with empty ID", hook: "run", body: `{"microvmId":""}`},
					{name: "run with numeric ID", hook: "run", body: `{"microvmId":42}`},
					{name: "run with conflicting IDs", hook: "run", body: `{"microvmId":"bad","microvmId":42}`},
					{name: "first run", hook: "run", body: `{"microvmId":"vm-A"}`, payloads: 1, id: "vm-A", uuid: "A"},
					{name: "suspend", hook: "suspend", payloads: 1},
					{name: "resume", hook: "resume", payloads: 2, id: "vm-A", uuid: "A"},
					{name: "repeated run", hook: "run", body: `{"microvmId":"vm-A"}`, payloads: 3, id: "vm-A", uuid: "A"},
					{name: "new instance", hook: "run", body: `{"microvmId":"vm-B"}`, payloads: 4, id: "vm-B", uuid: "B"},
					{name: "resume new instance", hook: "resume", payloads: 5, id: "vm-B", uuid: "B"},
				}
				uuids := map[string]string{}
				for _, step := range steps {
					post(step.hook, step.body, step.payloads)
					if step.payloads == 0 {
						assert.Empty(t, service.GetMetadata().ResourceID, "%s: no identity before a valid run", step.name)
					}
					if step.id == "" {
						continue
					}
					if step.name == "first run" {
						select {
						case <-collected:
						case <-time.After(5 * time.Second):
							t.Fatal("the first collection did not observe the /run identity")
						}
					}
					p.Provider.Callback(context.Background())
					payloads := serial.sent()
					require.Len(t, payloads, step.payloads, "%s: collection must not repeat a lifecycle report", step.name)
					payload := payloads[step.payloads-1]
					require.NotEmpty(t, payload.UUID, step.name)
					if want, seen := uuids[step.uuid]; seen {
						assert.Equal(t, want, payload.UUID, step.name)
					} else {
						assert.NotContains(t, uuids, payload.UUID, step.name)
						uuids[step.uuid] = payload.UUID
					}
					for key, expected := range map[string]string{
						"resource_id": step.id, "parent_resource_id": imageARN, "resource_name": "my-image",
						"region": "us-east-1", "aws_account_id": "123456789012", "workload_type": "aws_lambda_microvm",
						"runtime": "Python", "flavor": "serverless-init", "deployment_model": "in-container",
						"dd_service": "test-service", "dd_env": "test-env", "dd_version": "test-version", "dd_site": "datadoghq.eu",
						"report_reason": "startup",
					} {
						assert.Equal(t, expected, payload.Metadata[key], "%s: %s", step.name, key)
					}
					assert.Equal(t, step.id, srv.InstanceID(), step.name)
				}
				require.Len(t, lc.hooks, 1)
				require.NoError(t, lc.hooks[0].OnStop(context.Background()))
				assert.Len(t, serial.sent(), 5, "shutdown must not repeat the reported identity")
				if forwarding {
					assert.EqualValues(t, 11, forwarded.Load())
				} else {
					assert.Zero(t, forwarded.Load())
				}
			})
		}
	}
}

func TestMicroVMMetadataDisabled(t *testing.T) {
	t.Setenv(serverlessenv.MicroVMImageARNEnvVar, "arn:aws:lambda:us-east-1:123456789012:microvm-image:my-image")
	service := &cloudservice.MicroVM{}
	p, serial, lc := newMicroVMMetadataComponent(t, service, false, 0)
	service.MetadataReporter(p.Comp).ReportMetadata("vm-A")
	assert.Equal(t, "vm-A", service.GetMetadata().ResourceID)
	assert.Empty(t, serial.sent())
	assert.Nil(t, p.Provider.Callback, "disabled metadata must not register collection")
	assert.Empty(t, lc.hooks)
}
