// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	serverlessInitInventory "github.com/DataDog/datadog-agent/cmd/serverless-init/inventory"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/lifecycle"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	inventoryagent "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/def"
	inventoryagentimpl "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/impl"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessenv "github.com/DataDog/datadog-agent/pkg/serverless/env"
)

// Poll after every individual metadata update, including the temporary image
// resource ID, to force the interleavings the publication gate must suppress.
type microVMInventoryPollingComponent struct {
	inventoryagent.Component
	poll func()
}

func (c microVMInventoryPollingComponent) Set(key string, value interface{}) {
	c.Component.Set(key, value)
	if key != "report_reason" || value != "periodic" {
		c.poll()
	}
}

type microVMInventoryTelemetry struct{}

func (microVMInventoryTelemetry) Flush() {}
func (microVMInventoryTelemetry) AddEnhancedMetric(string, float64, metrics.MetricSource, float64, ...string) {
}

func TestMicroVMInventoryLifecycleReadiness(t *testing.T) {
	for _, forwarding := range []bool{false, true} {
		for _, delay := range []int{0, 3600} {
			t.Run(fmt.Sprintf("forwarding=%t/delay=%d", forwarding, delay), func(t *testing.T) {
				const imageARN = "arn:aws:lambda:us-east-1:123456789012:microvm-image:my-image"
				t.Setenv(serverlessenv.MicroVMImageARNEnvVar, imageARN)
				t.Setenv("DD_SERVERLESS_INVENTORY_RUNTIME", "python")
				conf := coreconfig.NewMock(t)
				for _, key := range []string{"serverless.inventory_enabled", "inventories_enabled", "enable_metadata_collection"} {
					conf.Set(key, true, configmodel.SourceAgentRuntime)
				}
				conf.Set("inventories_configuration_enabled", false, configmodel.SourceAgentRuntime)
				conf.Set("inventories_first_run_delay", delay, configmodel.SourceAgentRuntime)
				conf.Set("site", "datadoghq.eu", configmodel.SourceAgentRuntime)
				service := &cloudservice.MicroVM{}
				configureInventory(service)
				serial := &inventoryRecordingSerializer{}
				hostname, _ := hostnamemock.NewMock("inventory-test")
				uuid := serverlessInitInventory.NewInstanceUUID()
				imageUUID := uuid.Resolve()
				p := inventoryagentimpl.NewComponent(inventoryagentimpl.Requires{
					Config: conf, Log: logmock.New(t), Hostname: hostname, Serializer: serial,
					Capabilities: serverlessInitInventory.NewInstanceCapabilities(uuid),
				})
				require.NotNil(t, p.Provider.Callback)
				diagnostics := p.Comp.(interface{ GetAsJSON() ([]byte, error) })
				pollClosed := func() {
					before := len(serial.Payloads())
					p.Provider.Callback(context.Background())
					p.Comp.Submit()
					_, err := diagnostics.GetAsJSON()
					assert.EqualError(t, err, "inventory metadata is not ready")
					assert.Len(t, serial.Payloads(), before, "closed polls must not emit partial metadata")
				}
				pollClosed() // Initial image and a restored initial snapshot share this closed state.
				component := microVMInventoryPollingComponent{Component: p.Comp, poll: pollClosed}
				var published atomic.Int32
				var forwarded atomic.Int32
				var expectedPublications atomic.Int32
				var forwarder *lifecycle.Forwarder
				if forwarding {
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						assert.Equal(t, expectedPublications.Load(), published.Load(), "publication must finish before forwarding")
						forwarded.Add(1)
						w.WriteHeader(http.StatusAccepted)
					}))
					t.Cleanup(upstream.Close)
					forwarder = lifecycle.NewForwarder(upstream.Listener.Addr().(*net.TCPAddr).Port, 5*time.Second, 5*time.Second, 5*time.Second)
				}
				telemetry := microVMInventoryTelemetry{}
				srv := lifecycle.NewServer(0, telemetry, telemetry, nil, telemetry, nil,
					metrics.MetricSourceAWSMicroVMEnhanced, time.Second, lifecycle.NewNoopChildHandle(), forwarder,
					lifecycle.HookToggles{Run: true, Resume: true}, nil)
				srv.SetInventorySubmitter(lifecycle.InventorySubmitterFunc(func(id string) {
					serverlessInitInventory.PublishInstance(component, uuid, id, service, mode.Conf{SidecarMode: true}, conf,
						map[string]string{"service": "test-service", "env": "test-env", "version": "test-version"})
					published.Add(1)
				}))
				require.NoError(t, srv.ListenAndServe(func(err error) { assert.NoError(t, err) }))
				t.Cleanup(func() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					assert.NoError(t, srv.Stop(ctx))
				})
				client := &http.Client{Timeout: 5 * time.Second}
				post := func(hook, body string, expected int32) {
					t.Helper()
					expectedPublications.Store(expected)
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
					assert.Equal(t, expected, published.Load(), "publication must finish before the hook response")
				}
				post("ready", "", 0)
				post("validate", "", 0)
				post("resume", "", 0)
				for _, body := range []string{"", "{}", `{"microvmId":""}`, `{"microvmId":42}`, `{"microvmId":"bad","microvmId":42}`} {
					post("run", body, 0)
					pollClosed()
				}
				assert.Empty(t, serial.Payloads())
				assert.Equal(t, imageUUID, uuid.Resolve())

				var instanceAUUID string
				for index, step := range []struct{ hook, body, id string }{
					{"run", `{"microvmId":"vm-A"}`, "vm-A"},
					{"resume", "", "vm-A"},
					{"run", `{"microvmId":"vm-A"}`, "vm-A"},
					{"run", `{"microvmId":"vm-B"}`, "vm-B"},
					{"resume", "", "vm-B"},
				} {
					if index == 1 {
						post("suspend", "", 1)
						_, err := diagnostics.GetAsJSON()
						require.NoError(t, err, "same-instance suspend does not invalidate inventory")
					}
					before := len(serial.Payloads())
					post(step.hook, step.body, int32(index+1))
					require.Len(t, serial.Payloads(), before+1, "hook must immediately submit despite first-run delay")
					p.Provider.Callback(context.Background())
					for payloadIndex, data := range serial.Payloads()[before:] {
						var payload struct {
							UUID     string                 `json:"uuid"`
							Metadata map[string]interface{} `json:"agent_metadata"`
						}
						require.NoError(t, json.Unmarshal(data, &payload))
						require.NotEmpty(t, payload.UUID)
						assert.NotEqual(t, imageUUID, payload.UUID)
						assert.Equal(t, uuid.Resolve(), payload.UUID)
						if index == 0 {
							instanceAUUID = payload.UUID
						}
						if step.id == "vm-A" {
							assert.Equal(t, instanceAUUID, payload.UUID)
						} else {
							assert.NotEqual(t, instanceAUUID, payload.UUID)
						}
						for key, expected := range map[string]string{
							"resource_id": step.id, "parent_resource_id": imageARN, "resource_name": "my-image",
							"region": "us-east-1", "aws_account_id": "123456789012", "workload_type": "aws_lambda_microvm",
							"runtime": "python", "flavor": "serverless-init", "deployment_model": "sidecar",
							"dd_service": "test-service", "dd_env": "test-env", "dd_version": "test-version", "dd_site": "datadoghq.eu",
						} {
							assert.Equal(t, expected, payload.Metadata[key], key)
						}
						reason := "startup"
						if payloadIndex > 0 {
							reason = "periodic"
						}
						assert.Equal(t, reason, payload.Metadata["report_reason"])
					}
					assert.Equal(t, step.id, srv.InstanceID())
				}
				if forwarding {
					assert.EqualValues(t, 11, forwarded.Load())
				} else {
					assert.Zero(t, forwarded.Load())
				}
			})
		}
	}
}

func TestMicroVMInventoryDisabled(t *testing.T) {
	for _, disabled := range []string{"serverless.inventory_enabled", "inventories_enabled", "enable_metadata_collection"} {
		t.Run(disabled, func(t *testing.T) {
			conf := coreconfig.NewMock(t)
			for _, key := range []string{"serverless.inventory_enabled", "inventories_enabled", "enable_metadata_collection"} {
				conf.Set(key, key != disabled, configmodel.SourceAgentRuntime)
			}
			service := &cloudservice.MicroVM{}
			configureInventory(service)
			serial := &inventoryRecordingSerializer{}
			hostname, _ := hostnamemock.NewMock("inventory-test")
			uuid := serverlessInitInventory.NewInstanceUUID()
			p := inventoryagentimpl.NewComponent(inventoryagentimpl.Requires{
				Config: conf, Log: logmock.New(t), Hostname: hostname, Serializer: serial,
				Capabilities: serverlessInitInventory.NewInstanceCapabilities(uuid),
			})
			serverlessInitInventory.PublishInstance(p.Comp, uuid, "vm-A", service, mode.Conf{}, conf, nil)
			assert.Empty(t, serial.Payloads())
			assert.Empty(t, p.Comp.Get())
			assert.Nil(t, p.Provider.Callback, "readiness must not enable a disabled provider")
		})
	}
}
