// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package servicetype_test

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/discovery/servicetype"
)

func TestDetect(t *testing.T) {
	data := []struct {
		name        string
		ports       []uint16
		serviceType servicetype.ServiceType
	}{
		{
			name:        "redis",
			ports:       []uint16{9443},
			serviceType: servicetype.DB,
		},
		{
			name:        "mongo",
			ports:       []uint16{27017, 27018, 27019, 27020},
			serviceType: servicetype.DB,
		},
		{
			name:        "elastic",
			ports:       []uint16{9200},
			serviceType: servicetype.Storage,
		},
		{
			name:        "web",
			ports:       []uint16{80},
			serviceType: servicetype.FrontEnd,
		},
		{
			name:        "internal",
			ports:       []uint16{8080},
			serviceType: servicetype.WebService,
		},
	}
	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			serviceType := servicetype.Detect(d.ports, []uint16{})
			if serviceType != d.serviceType {
				t.Errorf("expected %v, got %v", d.serviceType, serviceType)
			}
		})
	}
}

func TestDetectUDPPorts(t *testing.T) {
	// When no TCP port matches, UDP ports should be checked
	serviceType := servicetype.Detect([]uint16{8080}, []uint16{5432})
	if serviceType != servicetype.DB {
		t.Errorf("expected %v from UDP port, got %v", servicetype.DB, serviceType)
	}
}

func TestDetectUDPQueue(t *testing.T) {
	serviceType := servicetype.Detect([]uint16{}, []uint16{9092})
	if serviceType != servicetype.Queue {
		t.Errorf("expected %v from UDP port, got %v", servicetype.Queue, serviceType)
	}
}

func TestDetectNoPorts(t *testing.T) {
	// No ports at all should default to WebService
	serviceType := servicetype.Detect([]uint16{}, []uint16{})
	if serviceType != servicetype.WebService {
		t.Errorf("expected %v with no ports, got %v", servicetype.WebService, serviceType)
	}
}

func TestDetectNilPorts(t *testing.T) {
	serviceType := servicetype.Detect(nil, nil)
	if serviceType != servicetype.WebService {
		t.Errorf("expected %v with nil ports, got %v", servicetype.WebService, serviceType)
	}
}

func TestDetectTCPTakesPrecedenceOverUDP(t *testing.T) {
	// TCP port should be matched first even if UDP has a different match
	serviceType := servicetype.Detect([]uint16{5432}, []uint16{9092})
	if serviceType != servicetype.DB {
		t.Errorf("expected %v (TCP match), got %v", servicetype.DB, serviceType)
	}
}
