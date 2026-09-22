// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gnmi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestPreviewMetricsCommandOneShot(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"gnmi", "preview-metrics", "--address", "router-1", "--once", "--timeout", "5s"},
		runPreviewMetrics,
		func(_ core.BundleParams, params *previewMetricsParams) {
			assert.Equal(t, "router-1", params.address)
			assert.True(t, params.once)
			assert.Equal(t, 5*time.Second, params.timeout)
			assert.True(t, params.useTLS)
		})
}
