// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package devicededuper

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/snmp"
	"github.com/stretchr/testify/assert"
)

func TestDeviceDeduper_Equal(t *testing.T) {
	now := time.Now().UnixMilli()

	tests := []struct {
		name     string
		device1  DeviceInfo
		device2  DeviceInfo
		expected bool
	}{
		{
			name: "same device",
			device1: DeviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			device2: DeviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			expected: true,
		},
		{
			name: "different name",
			device1: DeviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			device2: DeviceInfo{
				Name:        "device2",
				Description: "test device",
				BootTimeMs:  now,
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			expected: false,
		},
		{
			name: "different description",
			device1: DeviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			device2: DeviceInfo{
				Name:        "device1",
				Description: "different description",
				BootTimeMs:  now,
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			expected: false,
		},
		{
			name: "different sysObjectID",
			device1: DeviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			device2: DeviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				SysObjectID: "1.3.6.1.4.1.9.1.2",
			},
			expected: false,
		},
		{
			name: "different IP",
			device1: DeviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			device2: DeviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			expected: true,
		},
		{
			name: "boot time within tolerance",
			device1: DeviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			device2: DeviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now + uptimeDiffToleranceMs - 5,
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			expected: true,
		},
		{
			name: "boot time outside tolerance",
			device1: DeviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			device2: DeviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now + uptimeDiffToleranceMs + 5,
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.device1.equal(tt.device2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeviceDeduper_Contains(t *testing.T) {
	deduper := &deviceDeduperImpl{
		deviceInfos:    make([]DeviceInfo, 0),
		pendingDevices: make([]PendingDevice, 0),
		ipsCounter:     make(map[string]*atomic.Uint32),
	}

	now := time.Now().UnixMilli()
	device := DeviceInfo{
		Name:        "device1",
		Description: "test device",
		BootTimeMs:  now,
		SysObjectID: "1.3.6.1.4.1.9.1.1",
	}

	assert.False(t, deduper.contains(device))

	deduper.deviceInfos = append(deduper.deviceInfos, device)
	assert.True(t, deduper.contains(device))

	// Check for a different device
	differentDevice := DeviceInfo{
		Name:        "device2",
		Description: "another device",
		BootTimeMs:  now,
		SysObjectID: "1.3.6.1.4.1.9.1.2",
	}
	assert.False(t, deduper.contains(differentDevice))
}

func TestDeviceDeduper_MarkIPAsProcessed(t *testing.T) {
	deduper := &deviceDeduperImpl{
		deviceInfos:    make([]DeviceInfo, 0),
		pendingDevices: make([]PendingDevice, 0),
		ipsCounter:     make(map[string]*atomic.Uint32),
	}

	counter1 := &atomic.Uint32{}
	counter1.Store(2)
	deduper.ipsCounter["192.168.1.1"] = counter1

	counter2 := &atomic.Uint32{}
	counter2.Store(1)
	deduper.ipsCounter["192.168.1.2"] = counter2

	deduper.MarkIPAsProcessed("192.168.1.1")
	count := deduper.ipsCounter["192.168.1.1"].Load()
	assert.Equal(t, uint32(1), count)

	deduper.MarkIPAsProcessed("192.168.1.1")
	count = deduper.ipsCounter["192.168.1.1"].Load()
	assert.Equal(t, uint32(0), count)

	deduper.MarkIPAsProcessed("192.168.1.2")
	count = deduper.ipsCounter["192.168.1.2"].Load()
	assert.Equal(t, uint32(0), count)
}

func TestDeviceDeduper_InitIPCounter(t *testing.T) {
	config := snmp.ListenerConfig{
		Configs: []snmp.Config{
			{
				Network: "192.168.1.0/30",
				Authentications: []snmp.Authentication{
					{Community: "public"},
					{Community: "private"},
				},
				IgnoredIPAddresses: map[string]bool{
					"192.168.1.0": true,
					"192.168.1.3": true,
				},
			},
		},
	}

	deduper := NewDeviceDeduper(config)

	deduperImpl, ok := deduper.(*deviceDeduperImpl)
	assert.True(t, ok)

	count := deduperImpl.ipsCounter["192.168.1.1"].Load()
	assert.Equal(t, uint32(2), count)
	count = deduperImpl.ipsCounter["192.168.1.2"].Load()
	assert.Equal(t, uint32(2), count)

	_, ok = deduperImpl.ipsCounter["192.168.1.0"]
	assert.False(t, ok)
	_, ok = deduperImpl.ipsCounter["192.168.1.3"]
	assert.False(t, ok)
}

func TestDeviceDeduper_CheckPreviousIPs(t *testing.T) {
	deduper := &deviceDeduperImpl{
		deviceInfos:    make([]DeviceInfo, 0),
		pendingDevices: make([]PendingDevice, 0),
		ipsCounter:     make(map[string]*atomic.Uint32),
	}

	counter1 := &atomic.Uint32{}
	counter1.Store(0)
	deduper.ipsCounter["192.168.1.1"] = counter1 // Already processed

	counter2 := &atomic.Uint32{}
	counter2.Store(1)
	deduper.ipsCounter["192.168.1.2"] = counter2 // Not processed yet

	counter3 := &atomic.Uint32{}
	counter3.Store(0)
	deduper.ipsCounter["192.168.1.3"] = counter3 // Already processed

	counter4 := &atomic.Uint32{}
	counter4.Store(0)
	deduper.ipsCounter["192.168.1.4"] = counter4 // Already processed

	// Check if all previous IPs of 192.168.1.3 are discovered
	// Since 192.168.1.2 is not processed yet and it's less than 192.168.1.3, should return false
	result := deduper.checkPreviousIPs("192.168.1.3")
	assert.False(t, result)

	counter2.Store(0)

	result = deduper.checkPreviousIPs("192.168.1.3")
	assert.True(t, result)

	result = deduper.checkPreviousIPs("192.168.1.5")
	assert.True(t, result)
}

func TestDeviceDeduper_AddPendingDevice(t *testing.T) {
	deduper := &deviceDeduperImpl{
		deviceInfos:    make([]DeviceInfo, 0),
		pendingDevices: make([]PendingDevice, 0),
		ipsCounter:     make(map[string]*atomic.Uint32),
	}

	now := time.Now().UnixMilli()
	pendingDevice1 := PendingDevice{
		Config: snmp.Config{
			Network: "192.168.1.0/30",
			Authentications: []snmp.Authentication{
				{Community: "public"},
			},
		},
		Info: DeviceInfo{
			Name:        "device1",
			Description: "test device",
			BootTimeMs:  now,
			SysObjectID: "1.3.6.1.4.1.9.1.1",
		},
		AuthIndex:  0,
		WriteCache: true,
		IP:         "192.168.1.2",
	}

	pendingDevice2 := PendingDevice{
		Config: snmp.Config{
			Network: "192.168.1.0/30",
			Authentications: []snmp.Authentication{
				{Community: "public"},
			},
		},
		Info: DeviceInfo{
			Name:        "device2",
			Description: "another device",
			BootTimeMs:  now,
			SysObjectID: "1.3.6.1.4.1.9.1.2",
		},
		AuthIndex:  0,
		WriteCache: true,
		IP:         "192.168.1.3",
	}

	deduper.AddPendingDevice(pendingDevice1)
	assert.Len(t, deduper.pendingDevices, 1)

	deduper.AddPendingDevice(pendingDevice2)

	assert.Len(t, deduper.pendingDevices, 2)

	pendingDevice3 := PendingDevice{
		Config: snmp.Config{
			Network: "192.168.1.0/30",
			Authentications: []snmp.Authentication{
				{Community: "public"},
			},
		},
		Info: DeviceInfo{
			Name:        "device1",
			Description: "test device",
			BootTimeMs:  now,
			SysObjectID: "1.3.6.1.4.1.9.1.1",
		},
		AuthIndex:  0,
		WriteCache: true,
		IP:         "192.168.1.1",
	}

	deduper.AddPendingDevice(pendingDevice3)

	// We should now have device3 replacing device1, plus device2
	assert.Len(t, deduper.pendingDevices, 2)
	assert.Equal(t, pendingDevice2, deduper.pendingDevices[0])
	assert.Equal(t, pendingDevice3, deduper.pendingDevices[1])

	// Add service for same device but with higher IP - should be ignored
	pendingDevice4 := PendingDevice{
		Config: snmp.Config{
			Network: "192.168.1.0/30",
			Authentications: []snmp.Authentication{
				{Community: "public"},
			},
		},
		Info: DeviceInfo{
			Name:        "device2",
			Description: "another device",
			BootTimeMs:  now,
			SysObjectID: "1.3.6.1.4.1.9.1.2",
		},
		AuthIndex:  0,
		WriteCache: true,
		IP:         "192.168.1.4",
	}

	deduper.AddPendingDevice(pendingDevice4)

	assert.Len(t, deduper.pendingDevices, 2)
	assert.Equal(t, pendingDevice2, deduper.pendingDevices[0])
	assert.Equal(t, pendingDevice3, deduper.pendingDevices[1])
}

func TestDeviceDeduper_GetDedupedDevices(t *testing.T) {
	deduper := &deviceDeduperImpl{
		deviceInfos:    make([]DeviceInfo, 0),
		pendingDevices: make([]PendingDevice, 0),
		ipsCounter:     make(map[string]*atomic.Uint32),
	}

	now := time.Now().UnixMilli()

	device1 := DeviceInfo{
		Name:        "device1",
		Description: "test device",
		BootTimeMs:  now,
		SysObjectID: "1.3.6.1.4.1.9.1.1",
	}

	device2 := DeviceInfo{
		Name:        "device2",
		Description: "another device",
		BootTimeMs:  now,
		SysObjectID: "1.3.6.1.4.1.9.1.2",
	}

	pendingDevice1 := PendingDevice{
		Config: snmp.Config{
			Network: "192.168.1.0/30",
			Authentications: []snmp.Authentication{
				{Community: "public"},
			},
		},
		Info:       device1,
		AuthIndex:  0,
		WriteCache: true,
		IP:         "192.168.1.1",
	}

	pendingDevice2 := PendingDevice{
		Config: snmp.Config{
			Network: "192.168.1.0/30",
			Authentications: []snmp.Authentication{
				{Community: "public"},
			},
		},
		Info:       device2,
		AuthIndex:  0,
		WriteCache: true,
		IP:         "192.168.1.3",
	}

	deduper.AddPendingDevice(pendingDevice1)
	deduper.AddPendingDevice(pendingDevice2)

	// Set IP counter such that 192.168.1.1 has all previous IPs discovered
	// but 192.168.1.3 does not
	counter0 := &atomic.Uint32{}
	counter0.Store(0)
	deduper.ipsCounter["192.168.1.0"] = counter0 // Already scanned

	counter1 := &atomic.Uint32{}
	counter1.Store(0)
	deduper.ipsCounter["192.168.1.1"] = counter1 // Current IP

	counter2 := &atomic.Uint32{}
	counter2.Store(1)
	deduper.ipsCounter["192.168.1.2"] = counter2 // Pending IP

	counter3 := &atomic.Uint32{}
	counter3.Store(0)
	deduper.ipsCounter["192.168.1.3"] = counter3 // Current IP

	dedupedDevices := deduper.GetDedupedDevices()

	// Only device1 should be activated since it has all previous IPs discovered
	assert.Len(t, dedupedDevices, 1)
	assert.Equal(t, pendingDevice1, dedupedDevices[0])

	// device2 should still be pending
	assert.Len(t, deduper.pendingDevices, 1)
	assert.Equal(t, pendingDevice2, deduper.pendingDevices[0])

	// Now mark all IPs as scanned
	counter2.Store(0)

	// Deduplicate again
	dedupedDevices = deduper.GetDedupedDevices()

	// Now device2 should be activated
	assert.Len(t, dedupedDevices, 1)
	assert.Equal(t, pendingDevice2, dedupedDevices[0])

	// No more pending devices
	assert.Len(t, deduper.pendingDevices, 0)
}

func TestMinimumIP(t *testing.T) {
	tests := []struct {
		name     string
		ip1      string
		ip2      string
		expected string
	}{
		{
			name:     "ip1 is smaller",
			ip1:      "192.168.1.1",
			ip2:      "192.168.1.2",
			expected: "192.168.1.1",
		},
		{
			name:     "ip2 is smaller",
			ip1:      "192.168.1.2",
			ip2:      "192.168.1.1",
			expected: "192.168.1.1",
		},
		{
			name:     "ips are equal",
			ip1:      "192.168.1.1",
			ip2:      "192.168.1.1",
			expected: "192.168.1.1",
		},
		{
			name:     "different subnets",
			ip1:      "192.168.2.1",
			ip2:      "192.168.1.1",
			expected: "192.168.1.1",
		},
		{
			name:     "invalid ip1",
			ip1:      "invalid",
			ip2:      "192.168.1.1",
			expected: "",
		},
		{
			name:     "invalid ip2",
			ip1:      "192.168.1.1",
			ip2:      "invalid",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := minimumIP(tt.ip1, tt.ip2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewDeviceDeduper(t *testing.T) {
	config := snmp.ListenerConfig{
		Configs: []snmp.Config{
			{
				Network: "192.168.1.0/30",
				Authentications: []snmp.Authentication{
					{Community: "public"},
				},
			},
		},
	}

	deduper := NewDeviceDeduper(config)
	assert.NotNil(t, deduper)

	deduperImpl, ok := deduper.(*deviceDeduperImpl)
	assert.True(t, ok)
	assert.NotNil(t, deduperImpl.deviceInfos)
	assert.NotNil(t, deduperImpl.pendingDevices)

	var count int
	for range deduperImpl.ipsCounter {
		count++
	}
	assert.Equal(t, 4, count)
}

func TestDeviceDeduper_ResetCounters(t *testing.T) {
	config := snmp.ListenerConfig{
		Configs: []snmp.Config{
			{
				Network: "192.168.1.0/30",
				Authentications: []snmp.Authentication{
					{Community: "public"},
					{Community: "private"},
				},
				IgnoredIPAddresses: map[string]bool{
					"192.168.1.0": true,
				},
			},
		},
	}

	deduper := NewDeviceDeduper(config)
	deduperImpl, ok := deduper.(*deviceDeduperImpl)
	assert.True(t, ok)

	// Initially, all counters should be set to 2 (number of authentications)
	count := deduperImpl.ipsCounter["192.168.1.1"].Load()
	assert.Equal(t, uint32(2), count)
	count = deduperImpl.ipsCounter["192.168.1.2"].Load()
	assert.Equal(t, uint32(2), count)

	// Process some IPs
	deduper.MarkIPAsProcessed("192.168.1.1")
	deduper.MarkIPAsProcessed("192.168.1.2")

	count = deduperImpl.ipsCounter["192.168.1.1"].Load()
	assert.Equal(t, uint32(1), count)
	count = deduperImpl.ipsCounter["192.168.1.2"].Load()
	assert.Equal(t, uint32(1), count)

	// Add a discovered device
	now := time.Now().UnixMilli()
	deviceInfo := DeviceInfo{
		Name:        "device1",
		Description: "test device",
		BootTimeMs:  now,
		SysObjectID: "1.3.6.1.4.1.9.1.1",
	}
	deduperImpl.deviceInfos = append(deduperImpl.deviceInfos, deviceInfo)
	assert.Len(t, deduperImpl.deviceInfos, 1)

	// Reset counters
	deduper.ResetCounters()

	// Verify counters are reset to initial value
	count = deduperImpl.ipsCounter["192.168.1.1"].Load()
	assert.Equal(t, uint32(2), count)
	count = deduperImpl.ipsCounter["192.168.1.2"].Load()
	assert.Equal(t, uint32(2), count)

	// Verify device infos are cleared
	assert.Len(t, deduperImpl.deviceInfos, 0)
}
