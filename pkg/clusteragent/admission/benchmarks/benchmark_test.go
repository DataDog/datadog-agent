// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package benchmarks

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	agentsidecar "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/agent_sidecar"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation"
	configwebhook "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// BenchmarkAdmission compares request-to-response construction on the same host.
// Copy this file unchanged into the source baseline with its baseline primitive
// adapter. Cluster latency and server persistence require separate live checks.
func BenchmarkAdmission(b *testing.B) {
	for _, containers := range []int{1, 10, 100} {
		for _, shape := range []struct{ env, unknown int }{{5, 0}, {40, 16384}} {
			for _, scenario := range []string{"noop", "annotation", "resources", "toleration", "normalization", "config", "ssi", "sidecar"} {
				b.Run(fmt.Sprintf("%s/containers=%d/env=%d/unknown=%d", scenario, containers, shape.env, shape.unknown), func(b *testing.B) {
					raw := benchmarkPod(b, containers, shape.env, shape.unknown, scenario)
					var run func() ([]byte, error)
					switch scenario {
					case "config", "ssi", "sidecar":
						cfg := coreconfig.NewMockWithOverrides(b, map[string]any{
							"admission_controller.inject_config.mode":                                   "hostip",
							"apm_config.instrumentation.enabled":                                        true,
							"admission_controller.auto_instrumentation.gradual_rollout.enabled":         false,
							"admission_controller.agent_sidecar.provider":                               "fargate",
							"admission_controller.agent_sidecar.cluster_agent.tls_verification.enabled": false,
						})
						var hook admission.WebhookFunc
						switch scenario {
						case "config":
							filter, err := configwebhook.NewFilter(cfg)
							if err != nil {
								b.Fatal(err)
							}
							hook = configwebhook.NewWebhook(cfg, configwebhook.NewMutator(configwebhook.NewMutatorConfig(cfg), filter)).WebhookFunc()
						case "ssi":
							webhook, err := autoinstrumentation.NewAutoInstrumentation(cfg, nil, nil, nil, nil, nil)
							if err != nil {
								b.Fatal(err)
							}
							hook = webhook.WebhookFunc()
						case "sidecar":
							hook = agentsidecar.NewWebhook(cfg).WebhookFunc()
						}
						dry := true
						request := &admission.Request{Object: raw, Namespace: "benchmark", DryRun: &dry}
						run = func() ([]byte, error) {
							r := hook(request)
							if r.Result != nil && r.Result.Message != "" {
								return nil, fmt.Errorf("admission failed: %s", r.Result.Message)
							}
							return r.Patch, nil
						}
					default:
						run = func() ([]byte, error) { return runPrimitive(raw, scenario) }
					}
					_ = log.ChangeLogLevel(log.InfoLvl)
					wire, err := run()
					if err != nil {
						b.Fatal(err)
					}
					var ops []json.RawMessage
					if json.Unmarshal(wire, &ops) != nil {
						b.Fatal("invalid patch")
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := run(); err != nil {
							b.Fatal(err)
						}
					}
					b.StopTimer()
					b.ReportMetric(float64(len(wire)), "patch-bytes/op")
					b.ReportMetric(float64(len(ops)), "operations/op")
				})
			}
		}
	}
}

func benchmarkPod(b *testing.B, count, envCount, unknown int, scenario string) []byte {
	var containers []json.RawMessage
	for i := 0; i < count; i++ {
		var env []json.RawMessage
		for j := 0; j < envCount; j++ {
			env = append(env, json.RawMessage(fmt.Sprintf(`{"name":"ENV_%d","value":"value"}`, j)))
		}
		entries, _ := json.Marshal(env)
		containers = append(containers, json.RawMessage(fmt.Sprintf(`{"name":"app-%d","image":"app","env":%s,"resources":{"requests":{"cpu":"50m","memory":"128Mi"}}}`, i, entries)))
	}
	entries, _ := json.Marshal(containers)
	payload, _ := json.Marshal(strings.Repeat("x", unknown))
	extension := ""
	if unknown > 0 {
		extension = fmt.Sprintf(`,"custom":{"large":18446744073709551617,"precise":0.12345678901234567890123456789,"payload":%s}`, payload)
	}
	volumes := ""
	if scenario == "normalization" {
		volumes = `,"volumes":[{"name":"a","emptyDir":{}},{"name":"a","emptyDir":{}}]`
	}
	raw := []byte(fmt.Sprintf(`{"metadata":{"name":"benchmark","namespace":"benchmark","labels":{"admission.datadoghq.com/enabled":"true"},"annotations":{"admission.datadoghq.com/java-lib.version":"v1"}},"spec":{"containers":%s%s}%s}`, entries, volumes, extension))
	if !json.Valid(raw) {
		b.Fatal("invalid benchmark fixture")
	}
	return raw
}
