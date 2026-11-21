// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package module

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
)

type metricPacketWriter struct {
	mu      sync.Mutex
	packets []string
}

func (w *metricPacketWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.packets = append(w.packets, string(p))
	return len(p), nil
}

func (*metricPacketWriter) Close() error { return nil }

func (w *metricPacketWriter) take() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	packet := strings.Join(w.packets, "")
	w.packets = nil
	return strings.TrimSpace(packet)
}

type metricContainerStore struct {
	workloadmeta.Component
	container *workloadmeta.Container
	lookups   int
}

func (s *metricContainerStore) GetContainer(string) (*workloadmeta.Container, error) {
	s.lookups++
	if s.container == nil {
		return nil, errors.New("container not found")
	}
	return s.container, nil
}

func TestMetricProbeOriginPackets(t *testing.T) {
	t.Setenv("DD_ENV", "agent-env")
	t.Setenv("DD_SERVICE", "agent-service")
	t.Setenv("DD_VERSION", "agent-version")
	for _, entityID := range []string{"", "agent-pod"} {
		t.Run("DD_ENTITY_ID="+entityID, func(t *testing.T) {
			t.Setenv("DD_ENTITY_ID", entityID)
			for _, target := range []string{"host", "unknown", "pending-pod", "non-pod", "pod"} {
				t.Run(target, func(t *testing.T) {
					w := &metricPacketWriter{}
					client, err := ddgostatsd.NewWithWriter(w,
						ddgostatsd.WithChannelMode(),
						ddgostatsd.WithClientSideAggregation(),
						ddgostatsd.WithExtendedClientSideAggregation(),
						ddgostatsd.WithoutOriginDetection(),
						ddgostatsd.WithoutEntityID(),
						ddgostatsd.WithoutEnvTags(),
						ddgostatsd.WithoutTelemetry(),
					)
					require.NoError(t, err)
					t.Cleanup(func() { require.NoError(t, client.Close()) })

					store := &metricContainerStore{}
					if target != "unknown" {
						store.container = &workloadmeta.Container{}
					}
					switch target {
					case "pod":
						store.container.Owner = &workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "target-pod"}
					case "non-pod":
						store.container.Owner = &workloadmeta.EntityID{Kind: workloadmeta.KindECSTask, ID: "target-task"}
					}
					r := newMetricsReporter(client, time.Hour, nil, podLookup(store))
					t.Cleanup(r.close)
					runtimeID := procRuntimeID{service: "checkout", environment: "prod", version: "1.2.3"}
					if target != "host" {
						runtimeID.containerInfo = &process.ContainerInfo{ContainerID: "target-container"}
					}
					probe := metricProbe("probe", "COUNT", "calls", []string{
						"team:test", "dd.internal.entity_id:none", "dd.internal.entity_id:other-pod",
					}, reflect.Invalid)
					var state loader.MetricState
					r.register(1, &ir.Program{Probes: []*ir.Probe{probe}}, func() []loader.MetricState {
						return []loader.MetricState{state}
					}, runtimeID, nil)

					report := func(wantEntityID string) {
						state.Sum++
						state.Updates++
						r.reportAll()
						require.NoError(t, client.Flush())
						packet := w.take()
						require.Contains(t, packet, metricProbeNamePrefix+"calls:1|c|")
						_, tags, ok := strings.Cut(packet, "|#")
						require.True(t, ok, packet)
						tags, _, _ = strings.Cut(tags, "|")
						wantTags := []string{"team:test", "debugger.probeid:probe", "service:checkout", "env:prod", "version:1.2.3"}
						if wantEntityID != "" {
							wantTags = append(wantTags, "dd.internal.entity_id:"+wantEntityID)
						}
						require.ElementsMatch(t, wantTags, strings.Split(tags, ","), packet)
					}
					if target == "pod" {
						report("target-pod")
					} else {
						report("")
					}
					if target == "unknown" || target == "pending-pod" {
						store.container = &workloadmeta.Container{}
						report("")
						store.container.Owner = &workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "target-pod"}
						report("target-pod")
						require.Equal(t, 3, store.lookups)
					}
					lookups := store.lookups
					if target == "pod" || target == "unknown" || target == "pending-pod" {
						report("target-pod")
					} else {
						report("")
					}
					require.Equal(t, lookups, store.lookups, "resolved identities must not be looked up again")
				})
			}
		})
	}
}
