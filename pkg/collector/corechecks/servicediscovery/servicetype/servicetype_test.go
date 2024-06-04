// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicetype_test

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/servicetype"
)

func TestDetect(t *testing.T) {
	data := []struct {
		name        string
		serviceName string
		ports       []int
		serviceType servicetype.ServiceType
	}{
		{
			name:        "redis",
			serviceName: "redis",
			ports:       []int{9443},
			serviceType: servicetype.DB,
		},
		{
			name:        "mongo",
			serviceName: "mongo",
			ports:       []int{27017, 27018, 27019, 27020},
			serviceType: servicetype.DB,
		},
		{
			name:        "elastic",
			serviceName: "elastic",
			ports:       []int{9200},
			serviceType: servicetype.Storage,
		},
		{
			name:        "web",
			serviceName: "apache",
			ports:       []int{80},
			serviceType: servicetype.FrontEnd,
		},
		{
			name:        "internal",
			serviceName: "myService",
			ports:       []int{8080},
			serviceType: servicetype.WebService,
		},
	}
	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			serviceType := servicetype.Detect(d.serviceName, d.ports)
			if serviceType != d.serviceType {
				t.Errorf("expected %v, got %v", d.serviceType, serviceType)
			}
		})
	}
}
