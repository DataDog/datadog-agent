// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package listeners

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestStaticConfigSchedulesNUMAMonitoring(t *testing.T) {
	mock.New(t)
	systemProbeConfig := mock.NewSystemProbe(t)
	systemProbeConfig.Set("numa_monitoring.enabled", true, configmodel.SourceUnknown)

	services := make(chan Service, 16)
	listener := &StaticConfigListener{newService: services}
	listener.createServices()
	close(services)

	var found bool
	for service := range services {
		if service.GetServiceID() == "_numa_monitoring" {
			found = true
		}
	}
	require.True(t, found)
}
