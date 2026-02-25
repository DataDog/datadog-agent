// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package configstreamimpl

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetrynoops "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// TestClientConnectsAndReceivesStream verifies the following are met:
// 1. A client can connect and receive snapshot then updates
// 2. Ordered sequence IDs
// 3. Correct typed values
func TestClientConnectsAndReceivesStream(t *testing.T) {
	cfg := configmock.New(t)

	// Register keys first
	cfg.BindEnvAndSetDefault("test_string", "")
	cfg.BindEnvAndSetDefault("test_int", 0)
	cfg.BindEnvAndSetDefault("test_bool", false)
	cfg.BindEnvAndSetDefault("typed_string", "")
	cfg.BindEnvAndSetDefault("typed_int", 0)
	cfg.BindEnvAndSetDefault("typed_bool", true)
	cfg.BindEnvAndSetDefault("typed_float", 0.0)

	// Set initial values (OnUpdate callbacks will be registered when configstream starts)
	cfg.Set("test_string", "initial_value", model.SourceFile)
	cfg.Set("test_int", 42, model.SourceFile)
	cfg.Set("test_bool", true, model.SourceFile)

	mockLog := logmock.New(t)
	cs := newConfigStreamForTest(t, cfg, mockLog)

	// Subscribe to the stream
	// Note: session_id is only required at the gRPC server level (RAR-gated)
	// The component itself doesn't enforce this, so we can test without it
	req := &pb.ConfigStreamRequest{Name: "test-client"}
	eventChan, unsubscribe := cs.Subscribe(req)
	defer unsubscribe()

	t.Run("1. Snapshot received first", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		select {
		case event := <-eventChan:
			snapshot := event.GetSnapshot()
			require.NotNil(t, snapshot, "First event should be a snapshot")

			assert.Equal(t, int32(3), snapshot.SequenceId, "Snapshot should have sequence ID 3 (from initial config sets)")
			assert.Equal(t, "core-agent", snapshot.Origin, "Snapshot should have origin 'core-agent'")
			assert.NotEmpty(t, snapshot.Settings, "Snapshot should contain settings")

			// Verify we can find our test settings
			foundString := false
			foundInt := false
			foundBool := false

			for _, setting := range snapshot.Settings {
				if setting.Source != model.SourceFile.String() {
					continue
				}
				switch setting.Key {
				case "test_string":
					foundString = true
					assert.Equal(t, "initial_value", setting.Value.GetStringValue())
				case "test_int":
					foundInt = true
					assert.Equal(t, float64(42), setting.Value.GetNumberValue())
				case "test_bool":
					foundBool = true
					assert.Equal(t, true, setting.Value.GetBoolValue())
				}
			}

			assert.True(t, foundString, "Should find test_string in snapshot")
			assert.True(t, foundInt, "Should find test_int in snapshot")
			assert.True(t, foundBool, "Should find test_bool in snapshot")

		case <-ctx.Done():
			t.Fatal("Timeout waiting for snapshot")
		}
	})

	t.Run("2. Updates received with ordered sequence IDs", func(t *testing.T) {
		cfg.Set("test_string", "updated_value_1", model.SourceAgentRuntime)
		cfg.Set("test_string", "updated_value_2", model.SourceAgentRuntime)
		cfg.Set("test_int", 100, model.SourceAgentRuntime)

		updates := make([]*pb.ConfigUpdate, 0)
		timeout := time.After(2 * time.Second)

		for i := 0; i < 3; i++ {
			select {
			case event := <-eventChan:
				update := event.GetUpdate()
				if update != nil {
					updates = append(updates, update)
				}
			case <-timeout:
				break
			}
		}

		assert.GreaterOrEqual(t, len(updates), 1, "Should receive at least one update")
		for _, update := range updates {
			assert.NotEmpty(t, update.Origin, "Update should have origin field populated")
		}

		for i := 1; i < len(updates); i++ {
			prevSeqID := updates[i-1].SequenceId
			currSeqID := updates[i].SequenceId
			assert.Greater(t, currSeqID, prevSeqID,
				"Sequence IDs should be strictly increasing: prev=%d, curr=%d", prevSeqID, currSeqID)
		}

		found := false
		for _, update := range updates {
			if update.Setting.Key == "test_int" && update.Setting.Value.GetNumberValue() == 100 {
				found = true
				break
			}
		}
		assert.True(t, found, "Should find the test_int update with value 100")
	})

	t.Run("3. Correct typed values", func(t *testing.T) {
		cfg.Set("typed_string", "hello", model.SourceAgentRuntime)
		cfg.Set("typed_int", 999, model.SourceAgentRuntime)
		cfg.Set("typed_bool", false, model.SourceAgentRuntime)
		cfg.Set("typed_float", 3.14, model.SourceAgentRuntime)

		timeout := time.After(2 * time.Second)
		typedValues := make(map[string]interface{})

		for len(typedValues) < 4 {
			select {
			case event := <-eventChan:
				if update := event.GetUpdate(); update != nil {
					key := update.Setting.Key
					value := update.Setting.Value

					switch key {
					case "typed_string":
						typedValues[key] = value.GetStringValue()
					case "typed_int":
						typedValues[key] = value.GetNumberValue()
					case "typed_bool":
						typedValues[key] = value.GetBoolValue()
					case "typed_float":
						typedValues[key] = value.GetNumberValue()
					}
				}
			case <-timeout:
				break
			}
		}

		assert.Equal(t, "hello", typedValues["typed_string"])
		assert.Equal(t, float64(999), typedValues["typed_int"])
		assert.Equal(t, false, typedValues["typed_bool"])
		assert.InDelta(t, 3.14, typedValues["typed_float"], 0.001)
	})
}

// TestMultipleSubscribers verifies that multiple clients can subscribe simultaneously
func TestMultipleSubscribers(t *testing.T) {
	cfg := configmock.New(t)

	cfg.BindEnvAndSetDefault("multi_test", "initial")

	mockLog := logmock.New(t)
	cs := newConfigStreamForTest(t, cfg, mockLog)

	// Create 3 subscribers
	subs := make([]<-chan *pb.ConfigEvent, 3)
	unsubFuncs := make([]func(), 3)

	for i := 0; i < 3; i++ {
		req := &pb.ConfigStreamRequest{Name: "test-client"}
		eventChan, unsub := cs.Subscribe(req)
		subs[i] = eventChan
		unsubFuncs[i] = unsub
	}

	defer func() {
		for _, unsub := range unsubFuncs {
			unsub()
		}
	}()

	// Each subscriber should receive a snapshot
	for i, sub := range subs {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		select {
		case event := <-sub:
			assert.NotNil(t, event.GetSnapshot(), "Subscriber %d should receive snapshot", i)
		case <-ctx.Done():
			t.Fatalf("Subscriber %d did not receive snapshot", i)
		}
		cancel()
	}

	// Update config
	cfg.Set("multi_test", "updated", model.SourceAgentRuntime)

	// All subscribers should receive the update
	for i, sub := range subs {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		select {
		case event := <-sub:
			update := event.GetUpdate()
			if assert.NotNil(t, update, "Subscriber %d should receive update", i) {
				assert.Equal(t, "multi_test", update.Setting.Key)
			}
		case <-ctx.Done():
			t.Logf("Warning: Subscriber %d did not receive update in time", i)
		}
		cancel()
	}
}

// TestDiscontinuityResync verifies that rapid config updates are handled gracefully
// without panicking or deadlocking.
func TestDiscontinuityResync(t *testing.T) {
	cfg := configmock.New(t)
	cfg.BindEnvAndSetDefault("rapid_update", 0)

	mockLog := logmock.New(t)
	cs := newConfigStreamForTest(t, cfg, mockLog)

	req := &pb.ConfigStreamRequest{Name: "test-client"}
	eventChan, unsubscribe := cs.Subscribe(req)
	defer unsubscribe()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	select {
	case <-eventChan:
		// snapshot received
	case <-ctx.Done():
		t.Fatal("Timeout waiting for initial snapshot")
	}

	// Create a rapid series of updates (may cause discontinuity)
	for i := 0; i < 20; i++ {
		cfg.Set("rapid_update", i, model.SourceAgentRuntime)
	}

	// Drain the channel - we should either get all updates or a resync snapshot
	receivedSnapshot := false
	for {
		select {
		case event := <-eventChan:
			if event.GetSnapshot() != nil {
				receivedSnapshot = true
				t.Log("Received resync snapshot (expected behavior on discontinuity)")
			}
		case <-time.After(100 * time.Millisecond):
			// No more events
			goto done
		}
	}
done:
	if receivedSnapshot {
		t.Log("✓ Resync mechanism working correctly")
	}
}

// newConfigStreamForTest creates a config stream for testing without lifecycle
func newConfigStreamForTest(t *testing.T, cfg config.Component, logger log.Component) *configStream {
	telemetryComp := telemetrynoops.GetCompatComponent()
	reqs := Requires{
		Lifecycle: compdef.NewTestLifecycle(t), // Test lifecycle (hooks not executed)
		Config:    cfg,
		Log:       logger,
		Telemetry: telemetryComp,
	}
	provides := NewComponent(reqs)

	// Extract the underlying configStream
	// and start the run loop manually since lifecycle hooks are not executed
	cs := provides.Comp.(*configStream)
	go cs.run()

	return cs
}

// configInterceptor is a test-specific mock for the config component that allows
// intercepting and dropping OnUpdate calls to simulate discontinuities.
type configInterceptor struct {
	model.BuildableConfig
	realCallback      model.NotificationReceiver
	swallowNextUpdate bool
}

func (ci *configInterceptor) OnUpdate(cb model.NotificationReceiver) {
	ci.realCallback = cb
	ci.BuildableConfig.OnUpdate(func(setting string, source model.Source, oldValue, newValue interface{}, sequenceID uint64) {
		if ci.swallowNextUpdate {
			ci.swallowNextUpdate = false
			return
		}
		if ci.realCallback != nil {
			ci.realCallback(setting, source, oldValue, newValue, sequenceID)
		}
	})
}

func buildComponent(t *testing.T) (Provides, *configInterceptor) {
	lc := compdef.NewTestLifecycle(t)
	log := logmock.New(t)
	cfg := configmock.New(t)

	// Register keys used in tests
	cfg.BindEnvAndSetDefault("my.new.setting", "")
	cfg.BindEnvAndSetDefault("my.other.setting", "")
	cfg.BindEnvAndSetDefault("dropped.setting", "")
	cfg.BindEnvAndSetDefault("another.setting", 0)
	cfg.BindEnvAndSetDefault("logs_config.auto_multi_line_detection", true)
	cfg.BindEnvAndSetDefault("logs_config.use_compression", false)

	config := &configInterceptor{BuildableConfig: cfg}

	reqs := Requires{
		Lifecycle: lc,
		Log:       log,
		Config:    config,
		Telemetry: telemetrynoops.GetCompatComponent(),
	}

	provides := NewComponent(reqs)

	// Start the component's run loop
	err := lc.Start(context.Background())
	require.NoError(t, err)

	t.Cleanup(func() {
		lc.Stop(context.Background())
	})

	return provides, config
}

func TestConfigStream(t *testing.T) {
	t.Run("receives snapshot and updates", func(t *testing.T) {
		provides, configComp := buildComponent(t)

		eventsCh, unsubscribe := provides.Comp.Subscribe(&pb.ConfigStreamRequest{Name: "test-client"})
		defer unsubscribe()

		var event *pb.ConfigEvent
		select {
		case event = <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for initial snapshot")
		}
		require.NotNil(t, event)
		_, isSnapshot := event.GetEvent().(*pb.ConfigEvent_Snapshot)
		require.True(t, isSnapshot, "first event must be a snapshot")

		// Change a config value.
		configComp.Set("my.new.setting", "new_value", model.SourceAgentRuntime)

		// Verify we receive the update.
		select {
		case event = <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for config update")
		}
		require.NotNil(t, event)
		update, isUpdate := event.GetEvent().(*pb.ConfigEvent_Update)
		require.True(t, isUpdate, "second event must be an update")

		require.Equal(t, "my", update.Update.Setting.Key)
		require.Equal(t, model.SourceAgentRuntime.String(), update.Update.Setting.Source)
		require.Equal(t, map[string]interface{}{
			"new": map[string]interface{}{
				"setting": "new_value",
			},
		}, update.Update.Setting.Value.AsInterface())
	})
	t.Run("receives unset updates", func(t *testing.T) {
		provides, configComp := buildComponent(t)

		eventsCh, unsubscribe := provides.Comp.Subscribe(&pb.ConfigStreamRequest{Name: "test-client"})
		defer unsubscribe()

		var event *pb.ConfigEvent
		select {
		case event = <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for initial snapshot")
		}
		require.NotNil(t, event)
		_, isSnapshot := event.GetEvent().(*pb.ConfigEvent_Snapshot)
		require.True(t, isSnapshot, "first event must be a snapshot")

		// set a configuration value and layer it
		configComp.Set("my.new.setting", "original_value", model.SourceAgentRuntime)
		configComp.Set("my.new.setting", "new_value", model.SourceCLI)

		// verify we receive the update for the first set.
		select {
		case event = <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for config update")
		}
		require.NotNil(t, event)
		update, isUpdate := event.GetEvent().(*pb.ConfigEvent_Update)
		require.True(t, isUpdate, "second event must be an update")
		require.Equal(t, "my", update.Update.Setting.Key)
		require.Equal(t, model.SourceAgentRuntime.String(), update.Update.Setting.Source)
		require.Equal(t, map[string]interface{}{
			"new": map[string]interface{}{
				"setting": "original_value",
			},
		}, update.Update.Setting.Value.AsInterface())

		// verify we receive the update for the second set.
		select {
		case event = <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for config update")
		}
		require.NotNil(t, event)
		update, isUpdate = event.GetEvent().(*pb.ConfigEvent_Update)
		require.True(t, isUpdate, "third event must be an update")
		require.Equal(t, "my", update.Update.Setting.Key)
		require.Equal(t, model.SourceCLI.String(), update.Update.Setting.Source)
		require.Equal(t, map[string]interface{}{
			"new": map[string]interface{}{
				"setting": "new_value",
			},
		}, update.Update.Setting.Value.AsInterface())

		configComp.UnsetForSource("my.new.setting", model.SourceCLI)
		// verify we receive the update for the unset.
		select {
		case event = <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for config update")
		}
		require.NotNil(t, event)
		update, isUpdate = event.GetEvent().(*pb.ConfigEvent_Update)
		require.True(t, isUpdate, "unset event must be an update")

		// verify that this source has been unset for the top-level key.
		require.Equal(t, "my", update.Update.Setting.Key)
		require.Equal(t, model.SourceCLI.String(), update.Update.Setting.Source)
		require.Nil(t, update.Update.Setting.Value.AsInterface())
	})

	t.Run("resyncs with snapshot on discontinuity", func(t *testing.T) {
		provides, configComp := buildComponent(t)

		eventsCh, unsubscribe := provides.Comp.Subscribe(&pb.ConfigStreamRequest{Name: "test-client-discontinuity"})
		defer unsubscribe()

		// receive the initial snapshot
		var event *pb.ConfigEvent
		select {
		case event = <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for initial snapshot")
		}
		require.NotNil(t, event)

		// swallow the next update to create a discontinuity
		configComp.swallowNextUpdate = true
		configComp.Set("dropped.setting", "dropped_value", model.SourceAgentRuntime)

		// send another update that will be processed.
		configComp.Set("another.setting", 123, model.SourceAgentRuntime)

		// verify the next event we get is a snapshot for recovery, not the update.
		select {
		case event = <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for recovery snapshot")
		}
		require.NotNil(t, event)
		_, isSnapshot := event.GetEvent().(*pb.ConfigEvent_Snapshot)
		require.True(t, isSnapshot, "event after discontinuity must be a new snapshot")

		// verify the snapshot contains the dropped setting
		require.Equal(t, "dropped_value", configComp.Get("dropped.setting"))
	})

	t.Run("unsubscribe closes channel", func(t *testing.T) {
		provides, _ := buildComponent(t)
		eventsCh, unsubscribe := provides.Comp.Subscribe(&pb.ConfigStreamRequest{Name: "test-client-unsubscribe"})

		// receive the initial snapshot
		select {
		case event := <-eventsCh:
			require.NotNil(t, event)
			_, isSnapshot := event.GetEvent().(*pb.ConfigEvent_Snapshot)
			require.True(t, isSnapshot, "first event must be a snapshot")
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for snapshot")
		}

		unsubscribe()

		// verify the channel is closed
		select {
		case _, ok := <-eventsCh:
			require.False(t, ok, "channel should be closed after unsubscribe")
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for channel to close")
		}
	})

	t.Run("snapshot contains top-level values grouped by source for nested keys", func(t *testing.T) {
		provides, configComp := buildComponent(t)

		// Set nested config values with different sources
		configComp.Set("logs_config.auto_multi_line_detection", false, model.SourceFile)
		configComp.Set("logs_config.use_compression", true, model.SourceAgentRuntime)

		eventsCh, unsubscribe := provides.Comp.Subscribe(&pb.ConfigStreamRequest{Name: "test-client-nested"})
		defer unsubscribe()

		// Get the snapshot
		var event *pb.ConfigEvent
		select {
		case event = <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for initial snapshot")
		}
		require.NotNil(t, event)
		snapshot, isSnapshot := event.GetEvent().(*pb.ConfigEvent_Snapshot)
		require.True(t, isSnapshot, "first event must be a snapshot")

		// Build a map of source -> value for top-level key logs_config
		logsConfigBySource := make(map[string]interface{})
		for _, setting := range snapshot.Snapshot.Settings {
			if setting.Key == "logs_config" {
				logsConfigBySource[setting.Source] = setting.Value.AsInterface()
			}
		}

		require.Contains(t, logsConfigBySource, model.SourceFile.String())
		require.Equal(t, map[string]interface{}{
			"auto_multi_line_detection": false,
		}, logsConfigBySource[model.SourceFile.String()])

		require.Contains(t, logsConfigBySource, model.SourceAgentRuntime.String())
		require.Equal(t, map[string]interface{}{
			"use_compression": true,
		}, logsConfigBySource[model.SourceAgentRuntime.String()])

		for _, setting := range snapshot.Snapshot.Settings {
			require.False(t, strings.HasPrefix(setting.Key, "logs_config."),
				"snapshot should not contain flattened logs_config.* keys: got %q", setting.Key)
		}
	})

	t.Run("snapshot emits top-level map key when child keys contain dots", func(t *testing.T) {
		provides, configComp := buildComponent(t)

		configComp.Set("additional_endpoints", map[string][]string{
			"https://foo.datadoghq.com": {"api_key"},
		}, model.SourceFile)

		eventsCh, unsubscribe := provides.Comp.Subscribe(&pb.ConfigStreamRequest{Name: "test-client-additional-endpoints"})
		defer unsubscribe()

		var event *pb.ConfigEvent
		select {
		case event = <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for initial snapshot")
		}
		require.NotNil(t, event)
		snapshot, isSnapshot := event.GetEvent().(*pb.ConfigEvent_Snapshot)
		require.True(t, isSnapshot, "first event must be a snapshot")

		foundTopLevel := false
		for _, setting := range snapshot.Snapshot.Settings {
			if setting.Source == model.SourceFile.String() && setting.Key == "additional_endpoints" {
				foundTopLevel = true
				require.Equal(t, map[string]interface{}{
					"https://foo.datadoghq.com": []interface{}{"api_key"},
				}, setting.Value.AsInterface())
			}
			require.False(t, strings.HasPrefix(setting.Key, "additional_endpoints."),
				"snapshot should not contain flattened additional_endpoints.* keys: got %q", setting.Key)
		}

		require.True(t, foundTopLevel, "snapshot should contain top-level additional_endpoints setting for source=file")
	})

	t.Run("updates normalize dotted keys to top-level key", func(t *testing.T) {
		provides, configComp := buildComponent(t)

		eventsCh, unsubscribe := provides.Comp.Subscribe(&pb.ConfigStreamRequest{Name: "test-client-dotted-update"})
		defer unsubscribe()

		// drain initial snapshot
		select {
		case <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for initial snapshot")
		}

		configComp.Set("additional_endpoints.https://foo.datadoghq.com", []string{"api_key"}, model.SourceAgentRuntime)

		var event *pb.ConfigEvent
		select {
		case event = <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for config update")
		}

		update, isUpdate := event.GetEvent().(*pb.ConfigEvent_Update)
		require.True(t, isUpdate, "event must be an update")
		require.Equal(t, "additional_endpoints", update.Update.Setting.Key)
		require.Equal(t, model.SourceAgentRuntime.String(), update.Update.Setting.Source)

		// Value should be the source-layer top-level value, not a flattened leaf value.
		settingsBySource, _ := configComp.AllSettingsBySourceWithSequenceID()
		sourceSettingsRaw, ok := settingsBySource[model.SourceAgentRuntime]
		require.True(t, ok)
		sourceSettings, ok := sourceSettingsRaw.(map[string]interface{})
		require.True(t, ok)
		expectedValueRaw, ok := sourceSettings["additional_endpoints"]
		require.True(t, ok)

		expectedValue, err := sanitizeValue(expectedValueRaw)
		require.NoError(t, err)
		require.Equal(t, expectedValue, update.Update.Setting.Value.AsInterface())
	})

	t.Run("updates emit top-level map key when set directly with dotted child keys", func(t *testing.T) {
		provides, configComp := buildComponent(t)

		eventsCh, unsubscribe := provides.Comp.Subscribe(&pb.ConfigStreamRequest{Name: "test-client-map-update"})
		defer unsubscribe()

		// drain initial snapshot
		select {
		case <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for initial snapshot")
		}

		configComp.Set("additional_endpoints", map[string][]string{
			"https://foo.datadoghq.com": {"api_key"},
			"https://bar.datadoghq.com": {"bar_api_key"},
		}, model.SourceFile)

		var event *pb.ConfigEvent
		select {
		case event = <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for map update")
		}

		update, isUpdate := event.GetEvent().(*pb.ConfigEvent_Update)
		require.True(t, isUpdate, "event must be an update")
		require.Equal(t, "additional_endpoints", update.Update.Setting.Key)
		require.Equal(t, model.SourceFile.String(), update.Update.Setting.Source)
		require.Equal(t, map[string]interface{}{
			"https://foo.datadoghq.com": []interface{}{"api_key"},
			"https://bar.datadoghq.com": []interface{}{"bar_api_key"},
		}, update.Update.Setting.Value.AsInterface())
	})

	t.Run("updates include full top-level object for sibling keys in same source", func(t *testing.T) {
		provides, configComp := buildComponent(t)

		eventsCh, unsubscribe := provides.Comp.Subscribe(&pb.ConfigStreamRequest{Name: "test-client-sibling-update"})
		defer unsubscribe()

		// drain initial snapshot
		select {
		case <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for initial snapshot")
		}

		configComp.Set("my.new.setting", "new_value", model.SourceAgentRuntime)
		select {
		case <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for first sibling update")
		}

		configComp.Set("my.other.setting", "other_value", model.SourceAgentRuntime)

		var event *pb.ConfigEvent
		select {
		case event = <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for second sibling update")
		}

		update, isUpdate := event.GetEvent().(*pb.ConfigEvent_Update)
		require.True(t, isUpdate, "event must be an update")
		require.Equal(t, "my", update.Update.Setting.Key)
		require.Equal(t, model.SourceAgentRuntime.String(), update.Update.Setting.Source)
		require.Equal(t, map[string]interface{}{
			"new": map[string]interface{}{
				"setting": "new_value",
			},
			"other": map[string]interface{}{
				"setting": "other_value",
			},
		}, update.Update.Setting.Value.AsInterface())
	})

	t.Run("unsetting one sibling keeps remaining top-level object for source", func(t *testing.T) {
		provides, configComp := buildComponent(t)

		eventsCh, unsubscribe := provides.Comp.Subscribe(&pb.ConfigStreamRequest{Name: "test-client-sibling-unset"})
		defer unsubscribe()

		// drain initial snapshot
		select {
		case <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for initial snapshot")
		}

		configComp.Set("my.new.setting", "new_value", model.SourceAgentRuntime)
		select {
		case <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for first sibling update")
		}

		configComp.Set("my.other.setting", "other_value", model.SourceAgentRuntime)
		select {
		case <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for second sibling update")
		}

		configComp.UnsetForSource("my.new.setting", model.SourceAgentRuntime)

		var event *pb.ConfigEvent
		select {
		case event = <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for sibling unset update")
		}

		update, isUpdate := event.GetEvent().(*pb.ConfigEvent_Update)
		require.True(t, isUpdate, "event must be an update")
		require.Equal(t, "my", update.Update.Setting.Key)
		require.Equal(t, model.SourceAgentRuntime.String(), update.Update.Setting.Source)
		require.Equal(t, map[string]interface{}{
			"other": map[string]interface{}{
				"setting": "other_value",
			},
		}, update.Update.Setting.Value.AsInterface())
	})
}
