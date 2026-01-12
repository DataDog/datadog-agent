// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package configstreamimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"

	compdef "github.com/DataDog/datadog-agent/comp/def"
)

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
	config := &configInterceptor{BuildableConfig: configmock.New(t)}

	reqs := Requires{
		Lifecycle: lc,
		Log:       log,
		Config:    config,
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

		require.Equal(t, "my.new.setting", update.Update.Setting.Key)
		require.Equal(t, "new_value", update.Update.Setting.Value.GetStringValue())
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
		require.Equal(t, "original_value", update.Update.Setting.Value.GetStringValue())

		// verify we receive the update for the second set.
		select {
		case event = <-eventsCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for config update")
		}
		require.NotNil(t, event)
		update, isUpdate = event.GetEvent().(*pb.ConfigEvent_Update)
		require.True(t, isUpdate, "third event must be an update")
		require.Equal(t, "new_value", update.Update.Setting.Value.GetStringValue())

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

		// verify that the value has been unset and back to the original value.
		require.Equal(t, "my.new.setting", update.Update.Setting.Key)
		require.Equal(t, "original_value", update.Update.Setting.Value.GetStringValue())
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
}
