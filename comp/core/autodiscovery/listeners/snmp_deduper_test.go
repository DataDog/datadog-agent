// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package listeners

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/snmp"
	"github.com/stretchr/testify/assert"
)

func TestDeviceDeduper_Equal(t *testing.T) {
	now := time.Now().UnixMilli()

	tests := []struct {
		name     string
		device1  deviceInfo
		device2  deviceInfo
		expected bool
	}{
		{
			name: "same device",
			device1: deviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				IP:          "192.168.1.1",
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			device2: deviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				IP:          "192.168.1.1",
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			expected: true,
		},
		{
			name: "different name",
			device1: deviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				IP:          "192.168.1.1",
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			device2: deviceInfo{
				Name:        "device2",
				Description: "test device",
				BootTimeMs:  now,
				IP:          "192.168.1.1",
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			expected: false,
		},
		{
			name: "different description",
			device1: deviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				IP:          "192.168.1.1",
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			device2: deviceInfo{
				Name:        "device1",
				Description: "different description",
				BootTimeMs:  now,
				IP:          "192.168.1.1",
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			expected: false,
		},
		{
			name: "different sysObjectID",
			device1: deviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				IP:          "192.168.1.1",
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			device2: deviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				IP:          "192.168.1.1",
				SysObjectID: "1.3.6.1.4.1.9.1.2",
			},
			expected: false,
		},
		{
			name: "different IP",
			device1: deviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				IP:          "192.168.1.1",
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			device2: deviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				IP:          "192.168.1.2",
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			expected: true,
		},
		{
			name: "boot time within tolerance",
			device1: deviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				IP:          "192.168.1.1",
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			device2: deviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now + uptimeDiffTolerance - 5,
				IP:          "192.168.1.1",
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			expected: true,
		},
		{
			name: "boot time outside tolerance",
			device1: deviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now,
				IP:          "192.168.1.1",
				SysObjectID: "1.3.6.1.4.1.9.1.1",
			},
			device2: deviceInfo{
				Name:        "device1",
				Description: "test device",
				BootTimeMs:  now + uptimeDiffTolerance + 5,
				IP:          "192.168.1.1",
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

func TestDeviceDeduper_AddDevice(t *testing.T) {
	deduper := &deviceDeduperImpl{
		devices:         make([]deviceInfo, 0),
		pendingServices: make([]pendingService, 0),
	}

	device := deviceInfo{
		Name:        "device1",
		Description: "test device",
		BootTimeMs:  time.Now().UnixMilli(),
		IP:          "192.168.1.1",
		SysObjectID: "1.3.6.1.4.1.9.1.1",
	}

	deduper.addDevice(device)
	assert.Len(t, deduper.devices, 1)
	assert.Equal(t, device, deduper.devices[0])
}

func TestDeviceDeduper_Contains(t *testing.T) {
	deduper := &deviceDeduperImpl{
		devices:         make([]deviceInfo, 0),
		pendingServices: make([]pendingService, 0),
	}

	now := time.Now().UnixMilli()
	device := deviceInfo{
		Name:        "device1",
		Description: "test device",
		BootTimeMs:  now,
		IP:          "192.168.1.1",
		SysObjectID: "1.3.6.1.4.1.9.1.1",
	}

	assert.False(t, deduper.contains(device))

	deduper.addDevice(device)
	assert.True(t, deduper.contains(device))

	// Check for a different device
	differentDevice := deviceInfo{
		Name:        "device2",
		Description: "another device",
		BootTimeMs:  now,
		IP:          "192.168.1.3",
		SysObjectID: "1.3.6.1.4.1.9.1.2",
	}
	assert.False(t, deduper.contains(differentDevice))
}

func TestDeviceDeduper_RemoveIP(t *testing.T) {
	deduper := &deviceDeduperImpl{
		devices:         make([]deviceInfo, 0),
		pendingServices: make([]pendingService, 0),
	}

	deduper.ipsCounter.Store("192.168.1.1", 2)
	deduper.ipsCounter.Store("192.168.1.2", 1)

	result := deduper.removeIP("192.168.1.1")
	assert.False(t, result)
	count, _ := deduper.ipsCounter.Load("192.168.1.1")
	assert.Equal(t, 1, count)

	result = deduper.removeIP("192.168.1.1")
	assert.True(t, result)
	count, _ = deduper.ipsCounter.Load("192.168.1.1")
	assert.Equal(t, 0, count)

	result = deduper.removeIP("192.168.1.2")
	assert.True(t, result)
	count, _ = deduper.ipsCounter.Load("192.168.1.2")
	assert.Equal(t, 0, count)
}

func TestDeviceDeduper_InitIPCounter(t *testing.T) {
	config := snmp.ListenerConfig{
		Configs: []snmp.Config{
			{
				Network: "192.168.1.0/30", // 4 IPs: 192.168.1.0-192.168.1.3
				Authentications: []snmp.Authentication{
					{Community: "public"},
					{Community: "private"},
				},
				IgnoredIPAddresses: map[string]bool{
					"192.168.1.0": true, // Network address
					"192.168.1.3": true, // Broadcast address
				},
			},
		},
	}

	deduper := &deviceDeduperImpl{
		devices:         make([]deviceInfo, 0),
		pendingServices: make([]pendingService, 0),
	}
	deduper.initIPCounter(config)

	count, _ := deduper.ipsCounter.Load("192.168.1.1")
	assert.Equal(t, 2, count)
	count, _ = deduper.ipsCounter.Load("192.168.1.2")
	assert.Equal(t, 2, count)

	_, ok := deduper.ipsCounter.Load("192.168.1.0")
	assert.False(t, ok)
	_, ok = deduper.ipsCounter.Load("192.168.1.3")
	assert.False(t, ok)
}

func TestDeviceDeduper_CheckPreviousIPs(t *testing.T) {
	deduper := &deviceDeduperImpl{
		devices:         make([]deviceInfo, 0),
		pendingServices: make([]pendingService, 0),
	}

	deduper.ipsCounter.Store("192.168.1.1", 0) // Already processed
	deduper.ipsCounter.Store("192.168.1.2", 1) // Not processed yet
	deduper.ipsCounter.Store("192.168.1.3", 0) // Already processed
	deduper.ipsCounter.Store("192.168.1.4", 0) // Already processed

	// Check if all previous IPs of 192.168.1.3 are discovered
	// Since 192.168.1.2 is not processed yet and it's less than 192.168.1.3, should return false
	result := deduper.checkPreviousIPs("192.168.1.3")
	assert.False(t, result)

	deduper.ipsCounter.Store("192.168.1.2", 0)

	result = deduper.checkPreviousIPs("192.168.1.3")
	assert.True(t, result)

	result = deduper.checkPreviousIPs("192.168.1.5")
	assert.True(t, result)
}

func TestDeviceDeduper_AddPendingService(t *testing.T) {
	deduper := &deviceDeduperImpl{
		devices:         make([]deviceInfo, 0),
		pendingServices: make([]pendingService, 0),
	}

	now := time.Now().UnixMilli()
	device1 := deviceInfo{
		Name:        "device1",
		Description: "test device",
		BootTimeMs:  now,
		IP:          "192.168.1.2",
		SysObjectID: "1.3.6.1.4.1.9.1.1",
	}

	device2 := deviceInfo{
		Name:        "device2",
		Description: "another device",
		BootTimeMs:  now,
		IP:          "192.168.1.3",
		SysObjectID: "1.3.6.1.4.1.9.1.2",
	}

	service1 := &SNMPService{
		adIdentifier: "snmp",
		entityID:     "id1",
		deviceIP:     "192.168.1.2",
	}

	service2 := &SNMPService{
		adIdentifier: "snmp",
		entityID:     "id2",
		deviceIP:     "192.168.1.3",
	}

	deduper.addPendingService(pendingService{
		svc:        service1,
		device:     device1,
		authIndex:  0,
		writeCache: true,
	})
	assert.Len(t, deduper.pendingServices, 1)

	deduper.addPendingService(pendingService{
		svc:        service2,
		device:     device2,
		authIndex:  0,
		writeCache: true,
	})
	assert.Len(t, deduper.pendingServices, 2)

	// Add service for same device but with lower IP
	service3 := &SNMPService{
		adIdentifier: "snmp",
		entityID:     "id3",
		deviceIP:     "192.168.1.1",
	}
	device3 := deviceInfo{
		Name:        "device1",
		Description: "test device",
		BootTimeMs:  now,
		IP:          "192.168.1.1",
		SysObjectID: "1.3.6.1.4.1.9.1.1",
	}
	deduper.addPendingService(pendingService{
		svc:        service3,
		device:     device3,
		authIndex:  0,
		writeCache: true,
	})

	// We should now have device3 replacing device1, plus device2
	assert.Len(t, deduper.pendingServices, 2)
	assert.Equal(t, service2, deduper.pendingServices[0].svc)
	assert.Equal(t, service3, deduper.pendingServices[1].svc)

	// Add service for same device but with higher IP - should be ignored
	service4 := &SNMPService{
		adIdentifier: "snmp",
		entityID:     "id4",
		deviceIP:     "192.168.1.4",
	}
	device4 := deviceInfo{
		Name:        "device2",
		Description: "another device",
		BootTimeMs:  now,
		IP:          "192.168.1.4",
		SysObjectID: "1.3.6.1.4.1.9.1.2",
	}
	deduper.addPendingService(pendingService{
		svc:        service4,
		device:     device4,
		authIndex:  0,
		writeCache: true,
	})

	assert.Len(t, deduper.pendingServices, 2)
	assert.Equal(t, service2, deduper.pendingServices[0].svc)
	assert.Equal(t, service3, deduper.pendingServices[1].svc)
}

func TestDeviceDeduper_FlushPendingServices(t *testing.T) {
	deduper := &deviceDeduperImpl{
		devices:         make([]deviceInfo, 0),
		pendingServices: make([]pendingService, 0),
	}

	now := time.Now().UnixMilli()

	device1 := deviceInfo{
		Name:        "device1",
		Description: "test device",
		BootTimeMs:  now,
		IP:          "192.168.1.1",
		SysObjectID: "1.3.6.1.4.1.9.1.1",
	}

	device2 := deviceInfo{
		Name:        "device2",
		Description: "another device",
		BootTimeMs:  now,
		IP:          "192.168.1.3",
		SysObjectID: "1.3.6.1.4.1.9.1.2",
	}

	service1 := &SNMPService{
		adIdentifier: "snmp",
		entityID:     "id1",
		deviceIP:     "192.168.1.1",
	}

	service2 := &SNMPService{
		adIdentifier: "snmp",
		entityID:     "id2",
		deviceIP:     "192.168.1.3",
	}

	deduper.addPendingService(pendingService{
		svc:        service1,
		device:     device1,
		authIndex:  0,
		writeCache: true,
	})

	deduper.addPendingService(pendingService{
		svc:        service2,
		device:     device2,
		authIndex:  0,
		writeCache: true,
	})

	// Set IP counter such that 192.168.1.1 has all previous IPs discovered
	// but 192.168.1.3 does not
	deduper.ipsCounter.Store("192.168.1.0", 0) // Already scanned
	deduper.ipsCounter.Store("192.168.1.1", 0) // Current IP
	deduper.ipsCounter.Store("192.168.1.2", 1) // Pending IP
	deduper.ipsCounter.Store("192.168.1.3", 0) // Pending IP

	activatedServices := deduper.flushPendingServices()

	// Only service1 should be activated since it has all previous IPs discovered
	assert.Len(t, activatedServices, 1)
	assert.Equal(t, service1, activatedServices[0].svc)

	// service2 should still be pending
	assert.Len(t, deduper.pendingServices, 1)
	assert.Equal(t, service2, deduper.pendingServices[0].svc)

	// Now mark all IPs as scanned
	deduper.ipsCounter.Store("192.168.1.2", 0)

	// Flush again
	activatedServices = deduper.flushPendingServices()

	// Now service2 should be activated
	assert.Len(t, activatedServices, 1)
	assert.Equal(t, service2, activatedServices[0].svc)

	// No more pending services
	assert.Len(t, deduper.pendingServices, 0)
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

	deduper := newDeviceDeduper(config)
	assert.NotNil(t, deduper)

	deduperImpl, ok := deduper.(*deviceDeduperImpl)
	assert.True(t, ok)
	assert.NotNil(t, deduperImpl.devices)
	assert.NotNil(t, deduperImpl.pendingServices)

	var count int
	deduperImpl.ipsCounter.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	assert.Equal(t, count, 4)
}
