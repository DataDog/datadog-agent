// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx

package jmxfetch

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/stretchr/testify/require"
)

func TestInitConfigJavaOptions(t *testing.T) {
	j := NewJMXFetch(nil)

	var initConfig integration.Data = []byte(`java_options: -Xmx200m`)

	j.ConfigureFromInitConfig(initConfig)

	require.Contains(t, j.JavaOptions, "Xmx200m")
}

func TestConflictingInitConfigJavaOptions(t *testing.T) {
	j := NewJMXFetch(nil)

	var configOne integration.Data = []byte(`java_options: -Xmx200m`)
	var configTwo integration.Data = []byte(`java_options: -Xmx444m`)

	j.ConfigureFromInitConfig(configOne)
	j.ConfigureFromInitConfig(configTwo)

	// First config wins
	require.Contains(t, j.JavaOptions, "Xmx200m")
	require.NotContains(t, j.JavaOptions, "Xmx444m")
}

func TestConflictingInstanceJavaOptions(t *testing.T) {
	j := NewJMXFetch(nil)

	var configOne integration.Data = []byte(`java_options: -Xmx200m`)
	var configTwo integration.Data = []byte(`java_options: -Xmx444m`)

	j.ConfigureFromInstance(configOne)
	j.ConfigureFromInstance(configTwo)

	// First config wins
	require.Contains(t, j.JavaOptions, "Xmx200m")
	require.NotContains(t, j.JavaOptions, "Xmx444m")
}

func TestConflictingInstanceInitJavaOptions(t *testing.T) {
	j := NewJMXFetch(nil)

	var configOne integration.Data = []byte(`java_options: -Xmx200m`)
	var configTwo integration.Data = []byte(`java_options: -Xmx444m`)

	j.ConfigureFromInitConfig(configOne)
	j.ConfigureFromInstance(configTwo)

	// First config wins
	require.Contains(t, j.JavaOptions, "Xmx200m")
	require.NotContains(t, j.JavaOptions, "Xmx444m")
}
