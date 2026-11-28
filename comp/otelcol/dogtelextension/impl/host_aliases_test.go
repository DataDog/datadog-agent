// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package dogtelextensionimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"

	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	noopsimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

const testHostname = "i-0123456789abcdef0-my-cluster"

type fakeHostAliasesEnv struct {
	kubernetes bool
	pods       []*workloadmeta.KubernetesPod
	aliases    []string
}

func newTestHostAliasesSender(t *testing.T, env *fakeHostAliasesEnv) (*hostAliasesSender, *serializermock.MetricSerializer) {
	t.Helper()
	hostname, _ := hostnameinterface.NewMock(testHostname)
	s := serializermock.NewMetricSerializer(t)
	sender := newHostAliasesSender(configmock.NewMockWithOverrides(t, nil), logmock.New(t), hostname, s, nil)
	sender.isKubernetes = func() bool { return env.kubernetes }
	sender.listPods = func() []*workloadmeta.KubernetesPod { return env.pods }
	sender.getHostAliases = func(context.Context) []string { return env.aliases }
	return sender, s
}

// expectPayload makes the serializer mock record the JSON of each host
// metadata payload it receives.
func expectPayload(s *serializermock.MetricSerializer) *[]string {
	var payloads []string
	s.On("SendHostMetadata", mock.Anything).Run(func(args mock.Arguments) {
		b, err := args.Get(0).(marshaler.JSONMarshaler).MarshalJSON()
		if err != nil {
			panic(err)
		}
		payloads = append(payloads, string(b))
	}).Return(nil)
	return &payloads
}

func pod(name string, labels map[string]string, phase string) *workloadmeta.KubernetesPod {
	return &workloadmeta.KubernetesPod{
		EntityMeta: workloadmeta.EntityMeta{Name: name, Namespace: "datadog", Labels: labels},
		Phase:      phase,
	}
}

var (
	ddotPod  = pod("ddot-abcde", map[string]string{"app.kubernetes.io/name": "opentelemetry-collector"}, "Running")
	agentPod = pod("datadog-agent-xyz12", map[string]string{
		"agent.datadoghq.com/component": "agent",
		"app.kubernetes.io/component":   "agent",
	}, "Running")
)

func TestHostAliasesPayload(t *testing.T) {
	sender, s := newTestHostAliasesSender(t, &fakeHostAliasesEnv{
		// The kubelet alias is the hostname itself; cloud providers can
		// repeat one another.
		aliases: []string{testHostname, "i-0123456789abcdef0", "", "i-0123456789abcdef0", "custom-alias"},
	})
	payloads := expectPayload(s)

	sender.collect(context.Background())

	require.Len(t, *payloads, 1)
	assert.JSONEq(t, `{
		"internalHostname": "`+testHostname+`",
		"meta": {
			"hostname": "`+testHostname+`",
			"host_aliases": ["custom-alias", "i-0123456789abcdef0"]
		}
	}`, (*payloads)[0])
}

func TestHostAliasesNoneBesidesHostname(t *testing.T) {
	sender, s := newTestHostAliasesSender(t, &fakeHostAliasesEnv{aliases: []string{testHostname}})

	next := sender.collect(context.Background())

	s.AssertNotCalled(t, "SendHostMetadata", mock.Anything)
	assert.Equal(t, defaultHostAliasesEarly, roundToEarly(next), "the schedule continues")
}

func TestHostAliasesKubernetes(t *testing.T) {
	aliases := []string{"i-0123456789abcdef0"}

	t.Run("waits for workloadmeta to list the pods of the node", func(t *testing.T) {
		sender, s := newTestHostAliasesSender(t, &fakeHostAliasesEnv{kubernetes: true, aliases: aliases})

		assert.Equal(t, hostAliasesPodsRetry, sender.collect(context.Background()))
		s.AssertNotCalled(t, "SendHostMetadata", mock.Anything)
	})

	t.Run("sends without a core Agent on the node", func(t *testing.T) {
		sender, s := newTestHostAliasesSender(t, &fakeHostAliasesEnv{kubernetes: true, pods: []*workloadmeta.KubernetesPod{ddotPod}, aliases: aliases})
		payloads := expectPayload(s)

		sender.collect(context.Background())
		assert.Len(t, *payloads, 1)
	})

	t.Run("leaves the host to a core Agent on the node", func(t *testing.T) {
		sender, s := newTestHostAliasesSender(t, &fakeHostAliasesEnv{kubernetes: true, pods: []*workloadmeta.KubernetesPod{ddotPod, agentPod}, aliases: aliases})

		next := sender.collect(context.Background())

		s.AssertNotCalled(t, "SendHostMetadata", mock.Anything)
		assert.Equal(t, defaultHostAliasesEarly, roundToEarly(next), "checks again on the regular schedule")
	})

	t.Run("ignores a core Agent pod that isn't running", func(t *testing.T) {
		pending := pod("datadog-agent-xyz12", agentPod.Labels, "Pending")
		sender, s := newTestHostAliasesSender(t, &fakeHostAliasesEnv{kubernetes: true, pods: []*workloadmeta.KubernetesPod{ddotPod, pending}, aliases: aliases})
		payloads := expectPayload(s)

		sender.collect(context.Background())
		assert.Len(t, *payloads, 1)
	})

	t.Run("needs the Datadog label, not just the generic component one", func(t *testing.T) {
		other := pod("other-agent", map[string]string{"app.kubernetes.io/component": "agent"}, "Running")
		sender, s := newTestHostAliasesSender(t, &fakeHostAliasesEnv{kubernetes: true, pods: []*workloadmeta.KubernetesPod{ddotPod, other}, aliases: aliases})
		payloads := expectPayload(s)

		sender.collect(context.Background())
		assert.Len(t, *payloads, 1)
	})

	t.Run("sends again once the core Agent pod is gone", func(t *testing.T) {
		env := &fakeHostAliasesEnv{kubernetes: true, pods: []*workloadmeta.KubernetesPod{ddotPod, agentPod}, aliases: aliases}
		sender, s := newTestHostAliasesSender(t, env)
		payloads := expectPayload(s)

		sender.collect(context.Background())
		assert.Empty(t, *payloads)

		env.pods = []*workloadmeta.KubernetesPod{ddotPod}
		sender.collect(context.Background())
		assert.Len(t, *payloads, 1)
	})
}

func TestHostAliasesSchedule(t *testing.T) {
	sender, _ := newTestHostAliasesSender(t, &fakeHostAliasesEnv{})

	var intervals []time.Duration
	for range 5 {
		intervals = append(intervals, sender.nextInterval())
	}
	// 5 min, 15 min, then capped at 30 min, each within the 20% jitter
	for i, want := range []time.Duration{5 * time.Minute, 15 * time.Minute, 30 * time.Minute, 30 * time.Minute, 30 * time.Minute} {
		assert.InDelta(t, want, intervals[i], float64(want)*hostAliasesJitter, "interval %d", i)
		assert.LessOrEqual(t, intervals[i], defaultHostAliasesInterval, "interval %d", i)
	}
}

func TestHostAliasesIntervals(t *testing.T) {
	tests := []struct {
		name      string
		providers []map[string]interface{}
		early     time.Duration
		interval  time.Duration
	}{
		{
			name:     "defaults",
			early:    defaultHostAliasesEarly,
			interval: defaultHostAliasesInterval,
		},
		{
			name:      "host entry",
			providers: []map[string]interface{}{{"name": "resources", "interval": 60}, {"name": "host", "interval": 600, "early_interval": 120}},
			early:     2 * time.Minute,
			interval:  10 * time.Minute,
		},
		{
			name:      "interval below the early default",
			providers: []map[string]interface{}{{"name": "host", "interval": 120}},
			early:     2 * time.Minute,
			interval:  2 * time.Minute,
		},
		{
			name:      "interval out of bounds",
			providers: []map[string]interface{}{{"name": "host", "interval": 30}},
			early:     defaultHostAliasesEarly,
			interval:  defaultHostAliasesInterval,
		},
		{
			name:      "early interval above the interval",
			providers: []map[string]interface{}{{"name": "host", "interval": 600, "early_interval": 900}},
			early:     defaultHostAliasesEarly,
			interval:  10 * time.Minute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var overrides map[string]interface{}
			if tt.providers != nil {
				overrides = map[string]interface{}{"metadata_providers": tt.providers}
			}
			early, interval := hostAliasesIntervals(configmock.NewMockWithOverrides(t, overrides), logmock.New(t))
			assert.Equal(t, tt.early, early)
			assert.Equal(t, tt.interval, interval)
		})
	}
}

func TestStart_StandaloneMode_HostAliases(t *testing.T) {
	newExtension := func(t *testing.T, metadata bool) (*dogtelExtension, *serializermock.MetricSerializer) {
		hostname, _ := hostnameinterface.NewMock("test-host")
		s := serializermock.NewMetricSerializer(t)
		s.On("SendIterableSeries", mock.Anything).Return(nil)
		return &dogtelExtension{
			config: createDefaultConfig().(*Config),
			log:    logmock.New(t),
			coreConfig: configmock.NewMockWithOverrides(t, map[string]interface{}{
				"otel_standalone":            true,
				"enable_metadata_collection": metadata,
			}),
			serializer: s,
			hostname:   hostname,
			telemetry:  noopsimpl.GetCompatComponent(),
			ipc:        ipcmock.New(t),
			buildInfo:  component.BuildInfo{},
		}, s
	}

	t.Run("enabled", func(t *testing.T) {
		ext, s := newExtension(t, true)
		// The sender asks the real cloud providers for aliases, which CI
		// runners on a cloud may have.
		s.On("SendHostMetadata", mock.Anything).Return(nil).Maybe()

		require.NoError(t, ext.Start(context.Background(), nil))
		assert.NotNil(t, ext.hostAliases)
		require.NoError(t, ext.Shutdown(context.Background()))
	})

	t.Run("disabled", func(t *testing.T) {
		ext, _ := newExtension(t, false)

		require.NoError(t, ext.Start(context.Background(), nil))
		assert.Nil(t, ext.hostAliases)
		require.NoError(t, ext.Shutdown(context.Background()))
	})
}

// roundToEarly maps an interval drawn from the first backoff step to the
// early interval, removing the jitter.
func roundToEarly(d time.Duration) time.Duration {
	if d >= time.Duration(float64(defaultHostAliasesEarly)*(1-hostAliasesJitter)) &&
		d <= time.Duration(float64(defaultHostAliasesEarly)*(1+hostAliasesJitter)) {
		return defaultHostAliasesEarly
	}
	return d
}
