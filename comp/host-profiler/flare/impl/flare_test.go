// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package flareimpl implements the flareimpl
package flareimpl

import (
	"bytes"
	"strconv"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
)

var config = map[string]string{
	"key1": "value1",
	"key2": "value2",
	"key3": "value3",
}

func TestOTelExtFlareBuilder(t *testing.T) {
	// Override the response that the flare builder gets from the otel extension
	overrideConfigResponseTemplate := `{"config": {{.config}}}`
	tmpl, err := template.New("").Parse(overrideConfigResponseTemplate)
	require.NoError(t, err)
	b := &bytes.Buffer{}
	err = tmpl.Execute(b, map[string]string{
		"config": strconv.Quote(toJSON(config)),
	})
	require.NoError(t, err)

	overrideConfigResponse = b.String()

	// Fill the flare
	f := helpers.NewFlareBuilderMock(t, false)
	flareImpl := &flareImpl{}
	flareImpl.fillFlare(f)

	f.AssertFileContent(strconv.Quote(toJSON(config)), "host-profiler/runtime.cfg")
}
