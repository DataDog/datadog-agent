// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndm

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm/handler"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// fakeHandler is a Handler whose every answer the test controls.
type fakeHandler struct {
	key     string
	configs map[string][]integration.Config // path -> configs to return
	err     error

	renderedPaths []string
}

func (h *fakeHandler) Key() string { return h.key }

func (h *fakeHandler) Render(path string, _ json.RawMessage) ([]integration.Config, error) {
	h.renderedPaths = append(h.renderedPaths, path)
	if h.err != nil {
		return nil, h.err
	}
	return h.configs[path], nil
}

func newTestProvider(t *testing.T, handlers ...handler.Handler) *Provider {
	t.Helper()
	p, err := NewProvider(logmock.New(t), handlers)
	require.NoError(t, err)
	return p
}

func TestNewProviderReportsTheAutodiscoveryProviderName(t *testing.T) {
	p := newTestProvider(t)
	assert.Equal(t, names.NDMRemoteConfig, p.String())
}

func TestNewProviderRejectsTwoHandlersClaimingTheSameKey(t *testing.T) {
	_, err := NewProvider(logmock.New(t), []handler.Handler{
		&fakeHandler{key: "snmp"},
		&fakeHandler{key: "snmp"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "snmp", "the error names the contested key")
}

func TestNewProviderRejectsAHandlerWithNoKey(t *testing.T) {
	_, err := NewProvider(logmock.New(t), []handler.Handler{&fakeHandler{key: ""}})
	assert.Error(t, err)
}

func TestNewProviderIgnoresANilHandler(t *testing.T) {
	p, err := NewProvider(logmock.New(t), []handler.Handler{nil, &fakeHandler{key: "snmp"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"snmp"}, p.RegisteredKeys())
}

func TestNewProviderSortsTheRegisteredKeys(t *testing.T) {
	p := newTestProvider(t, &fakeHandler{key: "snmp"}, &fakeHandler{key: "autodiscovery"})
	assert.Equal(t, []string{"autodiscovery", "snmp"}, p.RegisteredKeys())
}

// recorder collects the apply states Update reports: the last status per
// path, and how many times each path was reported.
type recorder struct {
	states map[string]state.ApplyStatus
	counts map[string]int
}

func newRecorder() *recorder {
	return &recorder{states: map[string]state.ApplyStatus{}, counts: map[string]int{}}
}

func (r *recorder) callback(path string, status state.ApplyStatus) {
	r.states[path] = status
	r.counts[path]++
}

func rawConfig(body string) state.RawConfig {
	return state.RawConfig{Config: []byte(body)}
}

// drain reads every change currently queued on the provider's channel,
// skipping the empty value the channel is primed with.
func drain(t *testing.T, ch <-chan integration.ConfigChanges) []integration.ConfigChanges {
	t.Helper()

	var changes []integration.ConfigChanges
	for {
		select {
		case c := <-ch:
			if c.IsEmpty() {
				continue
			}
			changes = append(changes, c)
		case <-time.After(50 * time.Millisecond):
			return changes
		}
	}
}

func snmpConfig(name string) integration.Config {
	return integration.Config{
		Name:      "snmp",
		Source:    "ndm-remote-config:snmp",
		Instances: []integration.Data{integration.Data("ip_address: " + name)},
	}
}

func TestUpdateSchedulesAHandlersConfigsAndAcknowledgesThePath(t *testing.T) {
	h := &fakeHandler{key: "snmp", configs: map[string][]integration.Config{
		"path-a": {snmpConfig("10.0.0.1")},
	}}
	p := newTestProvider(t, h)
	ch := p.Stream(context.Background())
	rec := newRecorder()

	p.Update(map[string]state.RawConfig{"path-a": rawConfig(`{"snmp":{}}`)}, rec.callback)

	changes := drain(t, ch)
	require.Len(t, changes, 1)
	assert.Len(t, changes[0].Schedule, 1)
	assert.Empty(t, changes[0].Unschedule)
	assert.Equal(t, state.ApplyStateAcknowledged, rec.states["path-a"].State)
	assert.Empty(t, p.GetConfigErrors())
}

func TestUpdateReportsNoApplyStateForADocumentItDoesNotOwn(t *testing.T) {
	h := &fakeHandler{key: "snmp"}
	p := newTestProvider(t, h)
	ch := p.Stream(context.Background())
	rec := newRecorder()

	p.Update(map[string]state.RawConfig{
		"empty":      rawConfig(`{}`),
		"other-team": rawConfig(`{"debug-config-pct":10}`),
		"not-json":   rawConfig(`{`),
		"json-null":  rawConfig(`null`),
		"json-array": rawConfig(`[]`),
	}, rec.callback)

	assert.Empty(t, rec.states, "no apply state at all for an unowned document")
	assert.Empty(t, drain(t, ch))
	assert.Empty(t, h.renderedPaths)
}

func TestUpdateReportsAnErrorStateAndStillSchedulesTheSucceedingKey(t *testing.T) {
	snmp := &fakeHandler{key: "snmp", configs: map[string][]integration.Config{
		"path-a": {snmpConfig("10.0.0.1")},
	}}
	ad := &fakeHandler{key: "autodiscovery", err: errors.New("range is too large")}
	p := newTestProvider(t, snmp, ad)
	ch := p.Stream(context.Background())
	rec := newRecorder()

	p.Update(map[string]state.RawConfig{
		"path-a": rawConfig(`{"snmp":{},"autodiscovery":{}}`),
	}, rec.callback)

	changes := drain(t, ch)
	require.Len(t, changes, 1)
	assert.Len(t, changes[0].Schedule, 1, "the snmp key still schedules")
	assert.Equal(t, state.ApplyStateError, rec.states["path-a"].State)
	assert.Equal(t, "autodiscovery: range is too large", rec.states["path-a"].Error)
	assert.Equal(t, types.ErrorMsgSet{"autodiscovery: range is too large": {}}, p.GetConfigErrors()["path-a"])
}

func TestUpdateEmitsNothingForAnUnchangedSnapshot(t *testing.T) {
	h := &fakeHandler{key: "snmp", configs: map[string][]integration.Config{
		"path-a": {snmpConfig("10.0.0.1")},
	}}
	p := newTestProvider(t, h)
	ch := p.Stream(context.Background())
	rec := newRecorder()
	snapshot := map[string]state.RawConfig{"path-a": rawConfig(`{"snmp":{}}`)}

	p.Update(snapshot, rec.callback)
	require.Len(t, drain(t, ch), 1)

	p.Update(snapshot, rec.callback)

	assert.Empty(t, drain(t, ch), "an identical config set emits no schedule or unschedule churn")
	assert.Equal(t, state.ApplyStateAcknowledged, rec.states["path-a"].State)
}

func TestUpdateReplacesAPathsConfigsInOneChange(t *testing.T) {
	h := &fakeHandler{key: "snmp", configs: map[string][]integration.Config{
		"path-a": {snmpConfig("10.0.0.1")},
	}}
	p := newTestProvider(t, h)
	ch := p.Stream(context.Background())
	rec := newRecorder()

	p.Update(map[string]state.RawConfig{"path-a": rawConfig(`{"snmp":{}}`)}, rec.callback)
	require.Len(t, drain(t, ch), 1)

	h.configs["path-a"] = []integration.Config{snmpConfig("10.0.0.2")}
	p.Update(map[string]state.RawConfig{"path-a": rawConfig(`{"snmp":{"v":2}}`)}, rec.callback)

	changes := drain(t, ch)
	require.Len(t, changes, 1, "a snapshot emits exactly one ConfigChanges")
	require.Len(t, changes[0].Unschedule, 1)
	require.Len(t, changes[0].Schedule, 1)
	assert.Equal(t, "ip_address: 10.0.0.1", string(changes[0].Unschedule[0].Instances[0]))
	assert.Equal(t, "ip_address: 10.0.0.2", string(changes[0].Schedule[0].Instances[0]))
}

func TestUpdateUnschedulesAPathThatLeftTheSnapshot(t *testing.T) {
	h := &fakeHandler{key: "snmp", configs: map[string][]integration.Config{
		"path-a": {snmpConfig("10.0.0.1")},
	}}
	p := newTestProvider(t, h)
	ch := p.Stream(context.Background())
	rec := newRecorder()

	p.Update(map[string]state.RawConfig{"path-a": rawConfig(`{"snmp":{}}`)}, rec.callback)
	require.Len(t, drain(t, ch), 1)

	p.Update(map[string]state.RawConfig{}, rec.callback)

	changes := drain(t, ch)
	require.Len(t, changes, 1)
	assert.Len(t, changes[0].Unschedule, 1)
	assert.Empty(t, changes[0].Schedule)
}

func TestUpdateUnschedulesAKeyThatLeftADocument(t *testing.T) {
	snmp := &fakeHandler{key: "snmp", configs: map[string][]integration.Config{
		"path-a": {snmpConfig("10.0.0.1")},
	}}
	ad := &fakeHandler{key: "autodiscovery"}
	p := newTestProvider(t, snmp, ad)
	ch := p.Stream(context.Background())
	rec := newRecorder()

	p.Update(map[string]state.RawConfig{"path-a": rawConfig(`{"snmp":{},"autodiscovery":{}}`)}, rec.callback)
	require.Len(t, drain(t, ch), 1)

	p.Update(map[string]state.RawConfig{"path-a": rawConfig(`{"autodiscovery":{}}`)}, rec.callback)

	changes := drain(t, ch)
	require.Len(t, changes, 1)
	assert.Len(t, changes[0].Unschedule, 1)
}

func TestUpdateDropsTheConfigErrorsOfAPathThatLeftTheSnapshot(t *testing.T) {
	h := &fakeHandler{key: "snmp", err: errors.New("credential \"c1\" is not available")}
	p := newTestProvider(t, h)
	p.Stream(context.Background())
	rec := newRecorder()

	p.Update(map[string]state.RawConfig{"path-a": rawConfig(`{"snmp":{}}`)}, rec.callback)
	require.NotEmpty(t, p.GetConfigErrors())

	p.Update(map[string]state.RawConfig{}, rec.callback)

	assert.Empty(t, p.GetConfigErrors(), "a deleted path leaves no stale error behind")
}

func TestStreamStopsSendingOnceTheContextIsCancelled(t *testing.T) {
	h := &fakeHandler{key: "snmp", configs: map[string][]integration.Config{
		"path-a": {snmpConfig("10.0.0.1")},
	}}
	p := newTestProvider(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	ch := p.Stream(ctx)

	cancel()
	assert.Eventually(t, func() bool {
		_, open := <-ch
		return !open
	}, time.Second, 10*time.Millisecond)

	p.Update(map[string]state.RawConfig{"path-a": rawConfig(`{"snmp":{}}`)}, newRecorder().callback)
}

func TestUpdateUnschedulesWhenAKeyStaysButProducesNoConfigs(t *testing.T) {
	h := &fakeHandler{key: "snmp", configs: map[string][]integration.Config{
		"path-a": {snmpConfig("10.0.0.1")},
	}}
	p := newTestProvider(t, h)
	ch := p.Stream(context.Background())
	rec := newRecorder()

	p.Update(map[string]state.RawConfig{"path-a": rawConfig(`{"snmp":{}}`)}, rec.callback)
	require.Len(t, drain(t, ch), 1)

	h.configs["path-a"] = nil
	p.Update(map[string]state.RawConfig{"path-a": rawConfig(`{"snmp":{"v":2}}`)}, rec.callback)

	changes := drain(t, ch)
	require.Len(t, changes, 1)
	assert.Len(t, changes[0].Unschedule, 1, "the configs it no longer produces are unscheduled")
	assert.Empty(t, changes[0].Schedule)
	assert.Equal(t, state.ApplyStateAcknowledged, rec.states["path-a"].State)
}

func TestUpdateUnschedulesWhenAKeyStaysButItsHandlerFails(t *testing.T) {
	h := &fakeHandler{key: "snmp", configs: map[string][]integration.Config{
		"path-a": {snmpConfig("10.0.0.1")},
	}}
	p := newTestProvider(t, h)
	ch := p.Stream(context.Background())
	rec := newRecorder()

	p.Update(map[string]state.RawConfig{"path-a": rawConfig(`{"snmp":{}}`)}, rec.callback)
	require.Len(t, drain(t, ch), 1)

	h.err = errors.New("credential is not available")
	p.Update(map[string]state.RawConfig{"path-a": rawConfig(`{"snmp":{"v":2}}`)}, rec.callback)

	changes := drain(t, ch)
	require.Len(t, changes, 1)
	assert.Len(t, changes[0].Unschedule, 1, "the configs it no longer produces are unscheduled")
	assert.Empty(t, changes[0].Schedule)
	assert.Equal(t, state.ApplyStateError, rec.states["path-a"].State)
	assert.Equal(t, types.ErrorMsgSet{"snmp: credential is not available": {}}, p.GetConfigErrors()["path-a"])
}

func TestUpdateAndGetConfigErrorsAreSafeToCallConcurrently(t *testing.T) {
	const iterations = 300

	failure := errors.New("credential is not available")
	expected := types.ErrorMsgSet{"snmp: " + failure.Error(): {}}

	h := &fakeHandler{key: "snmp", configs: map[string][]integration.Config{
		"path-a": {snmpConfig("10.0.0.1")},
	}}
	p := newTestProvider(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := p.Stream(ctx)

	stop := make(chan struct{})
	var wg, drainer sync.WaitGroup

	// A reader has to keep the channel moving or Update blocks on it.
	drainer.Add(1)
	go func() {
		defer drainer.Done()
		for {
			select {
			case <-ch:
			case <-stop:
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		rec := newRecorder()
		for i := range iterations {
			if i%2 == 0 {
				h.err = nil
			} else {
				h.err = failure
			}
			p.Update(map[string]state.RawConfig{"path-a": rawConfig(`{"snmp":{}}`)}, rec.callback)
		}
	}()

	var (
		observations int
		unexpected   []map[string]types.ErrorMsgSet
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range iterations {
			got := p.GetConfigErrors()
			observations++
			if len(got) == 0 {
				continue
			}
			if len(got) != 1 || !assert.ObjectsAreEqual(expected, got["path-a"]) {
				unexpected = append(unexpected, got)
			}
		}
	}()

	wg.Wait()
	close(stop)
	drainer.Wait()

	assert.Equal(t, iterations, observations, "the reader actually ran against the writer")
	assert.Empty(t, unexpected, "every observed error map was either empty or exactly one path's complete error set")

	// The last iteration is odd, so the failing snapshot is the one that stands.
	assert.Equal(t, map[string]types.ErrorMsgSet{"path-a": expected}, p.GetConfigErrors())
}

func TestUpdateBeforeStreamWaitsInTheChannel(t *testing.T) {
	h := &fakeHandler{key: "snmp", configs: map[string][]integration.Config{
		"path-a": {snmpConfig("10.0.0.1")},
	}}
	p := newTestProvider(t, h)
	rec := newRecorder()

	p.Update(map[string]state.RawConfig{"path-a": rawConfig(`{"snmp":{}}`)}, rec.callback)

	ch := p.Stream(context.Background())

	changes := drain(t, ch)
	require.Len(t, changes, 1)
	require.Len(t, changes[0].Schedule, 1)
	assert.Equal(t, "ip_address: 10.0.0.1", string(changes[0].Schedule[0].Instances[0]))
	assert.Equal(t, []string{"path-a"}, h.renderedPaths)
	assert.Equal(t, state.ApplyStateAcknowledged, rec.states["path-a"].State)
	assert.Equal(t, 1, rec.counts["path-a"])
}
