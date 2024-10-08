// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package servicetype_test

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/servicetype"
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
			serviceType := servicetype.Detect(d.ports)
			if serviceType != d.serviceType {
				t.Errorf("expected %v, got %v", d.serviceType, serviceType)
			}
		})
	}
}
