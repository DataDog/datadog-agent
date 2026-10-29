// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddot

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func TestModes(t *testing.T) {
	tests := []struct {
		name                string
		flavor              string
		standalone          bool
		useKubeletCollector bool
		wantStandalone      bool
		wantKubeletDisabled bool
	}{
		{name: "otel-agent standalone with the nodefilter collector", flavor: flavor.OTelAgent, standalone: true, wantStandalone: true, wantKubeletDisabled: true},
		{name: "otel-agent standalone with the kubelet collector", flavor: flavor.OTelAgent, standalone: true, useKubeletCollector: true, wantStandalone: true},
		{name: "otel-agent connected", flavor: flavor.OTelAgent},
		{name: "other flavors", flavor: flavor.DefaultAgent, standalone: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flavor.SetTestFlavor(t, tt.flavor)
			cfg := configmock.New(t)
			cfg.SetInTest("otel_standalone", tt.standalone)
			cfg.SetInTest("otelcollector.standalone.use_kubelet_collector", tt.useKubeletCollector)

			assert.Equal(t, tt.wantStandalone, IsStandalone(cfg))
			assert.Equal(t, tt.wantKubeletDisabled, KubeletDisabled(cfg))
		})
	}
}
