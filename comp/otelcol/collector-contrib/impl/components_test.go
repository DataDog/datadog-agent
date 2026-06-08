// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectorcontribimpl

import (
	"testing"

	dockerstatsreceiver "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/dockerstatsreceiver"
	kubeletstatsreceiver "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kubeletstatsreceiver"
	podmanreceiver "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/podmanreceiver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponents_IncludesNewReceivers(t *testing.T) {
	factories, err := components()
	require.NoError(t, err)

	_, found := factories.Receivers[dockerstatsreceiver.NewFactory().Type()]
	assert.True(t, found, "dockerstats receiver should be registered")

	_, found = factories.Receivers[kubeletstatsreceiver.NewFactory().Type()]
	assert.True(t, found, "kubeletstats receiver should be registered")

	_, found = factories.Receivers[podmanreceiver.NewFactory().Type()]
	assert.True(t, found, "podman receiver should be registered")
}
