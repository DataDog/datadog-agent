// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package hostsysteminfoimpl

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCaptureHardwareProjectionOwnsFieldsWithoutChangingNativePayload(t *testing.T) {
	p := &Payload{Hostname: "native-host", UUID: "native-uuid", Timestamp: 123456789, Metadata: &hostSystemInfoMetadata{
		Manufacturer: "Example Manufacturer", ModelNumber: "Example Model Number", SerialNumber: "private-serial",
		ModelName: "Example Model", ChassisType: "Laptop", Identifier: "private-identifier",
	}}
	before, err := p.MarshalJSON()
	require.NoError(t, err)
	at := time.Now()
	p.SetCaptureInventorySchedule(at, time.Hour)
	owned := p.CopyCaptureInventory()
	require.Equal(t, telemetrycapture.HostSystemInfo, p.CaptureInventoryStream())
	require.GreaterOrEqual(t, p.CaptureInventorySize(), telemetrycapture.PayloadSize(telemetrycapture.Payload{Inventory: owned}))
	require.Nil(t, owned.Agent)
	require.Nil(t, owned.Host)
	require.Equal(t, p.Timestamp, owned.Timestamp)
	copyFields := []string{owned.SystemInfo.Manufacturer, owned.SystemInfo.ModelNumber, owned.SystemInfo.SerialNumber,
		owned.SystemInfo.ModelName, owned.SystemInfo.ChassisType, owned.SystemInfo.Identifier, owned.Hostname, owned.UUID}
	for i, source := range []string{p.Metadata.Manufacturer, p.Metadata.ModelNumber, p.Metadata.SerialNumber,
		p.Metadata.ModelName, p.Metadata.ChassisType, p.Metadata.Identifier, p.Hostname, p.UUID} {
		require.Equal(t, source, copyFields[i])
		require.False(t, unsafe.StringData(source) == unsafe.StringData(copyFields[i]), "capture retains borrowed string storage")
	}
	after, err := p.MarshalJSON()
	require.NoError(t, err)
	require.Equal(t, before, after, "capture schedule and copying must not change production JSON")
	encoded, err := json.Marshal(owned)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(encoded))
	p.Metadata.SerialNumber, p.Hostname = "changed", "changed"
	require.Equal(t, "private-serial", owned.SystemInfo.SerialNumber)
	require.Equal(t, "native-host", owned.Hostname)
	collectedAt, cadence := p.CaptureInventorySchedule()
	require.Equal(t, at, collectedAt)
	require.Equal(t, time.Hour, cadence)
	require.Nil(t, (&Payload{}).CopyCaptureInventory())
}

type captureLifecycleHooks struct{ hooks []compdef.Hook }

func (l *captureLifecycleHooks) Append(hook compdef.Hook) { l.hooks = append(l.hooks, hook) }

func TestCaptureHardwareReadinessFollowsNativeProviderAndShutdown(t *testing.T) {
	for _, mode := range []string{"end_user_device", "full"} {
		t.Run(mode, func(t *testing.T) {
			m := telemetrycapture.NewManager("core-agent", "fixture", "fixture")
			t.Cleanup(m.Close)
			lc := &captureLifecycleHooks{}
			hostname, _ := hostnamemock.NewMock(hostnamemock.MockHostname("test-host"))
			s := serializermock.NewMetricSerializer(t)
			p := NewComponent(Requires{Lc: lc, CaptureManager: m, Log: logmock.New(t), Hostname: hostname, Serializer: s,
				Config: config.NewMockWithOverrides(t, map[string]any{"infrastructure_mode": mode, "inventories_first_run_delay": 0})})
			h := p.Comp.(*hostSystemInfo)
			require.Empty(t, m.Status().Capabilities, "construction is not producer readiness")
			require.Equal(t, time.Hour, h.MinInterval)
			require.Equal(t, time.Hour, h.MaxInterval)
			require.Len(t, lc.hooks, 1)
			if mode != "end_user_device" || (runtime.GOOS != "darwin" && runtime.GOOS != "windows") {
				require.Nil(t, p.Provider.Callback)
				require.NoError(t, lc.hooks[0].OnStop(context.Background()))
				return
			}
			// Exercise normal native collection without printing any hardware values.
			s.On("SendMetadata", mock.Anything).Run(func(args mock.Arguments) {
				require.Empty(t, m.Status().Capabilities, "readiness precedes successful delivery submission")
				payload := args.Get(0).(*Payload)
				at, cadence := payload.CaptureInventorySchedule()
				require.Equal(t, h.LastCollect, at)
				require.Equal(t, time.Hour, cadence)
			}).Return(nil).Once()
			require.Equal(t, time.Hour, p.Provider.Callback(context.Background()))
			require.Equal(t, []telemetrycapture.Capability{{Stream: telemetrycapture.HostSystemInfo, Cadence: time.Hour}}, m.Status().Capabilities)
			last := h.LastCollect
			h.Refresh()
			control := telemetrycapture.Control{ProtocolVersion: telemetrycapture.ProtocolVersion, SessionID: "fresh-hardware-session"}
			_, err := m.Prepare(telemetrycapture.PrepareRequest{Control: control, Streams: []telemetrycapture.Stream{telemetrycapture.HostSystemInfo}})
			require.NoError(t, err)
			active, err := m.Activate(control)
			require.NoError(t, err)
			s.On("SendMetadata", mock.Anything).Run(func(args mock.Arguments) {
				payload := args.Get(0).(*Payload)
				at, cadence := payload.CaptureInventorySchedule()
				require.False(t, at.Before(active.ActivatedAt))
				require.GreaterOrEqual(t, payload.Timestamp, at.UnixNano())
				require.Equal(t, time.Hour, cadence)
				require.NotNil(t, payload.CopyCaptureInventory(), "fresh normal payload supports the existing serializer tee")
			}).Return(nil).Once()
			for range 2 {
				_, err = m.RequestHostSystemInfo(context.Background(), control)
				require.NoError(t, err)
			}
			require.Equal(t, last, h.LastCollect)
			require.True(t, h.RefreshTriggered())
			require.Equal(t, time.Hour, h.MinInterval)
			require.Equal(t, time.Hour, h.MaxInterval)
			require.NoError(t, lc.hooks[0].OnStop(context.Background()))
			require.Empty(t, m.Status().Capabilities)
			// A normal collection finishing after shutdown still submits normally,
			// but cannot restore the capture capability.
			h.LastCollect = time.Now().Add(-2 * time.Hour)
			s.On("SendMetadata", mock.Anything).Return(nil).Once()
			require.Equal(t, time.Hour, p.Provider.Callback(context.Background()))
			require.Empty(t, m.Status().Capabilities)
		})
	}
}
