// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package v5

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/py"
	"github.com/DataDog/datadog-agent/pkg/metadata/externalhost"
	python "github.com/sbinet/go-python"
	"github.com/stretchr/testify/assert"
)

// Setup the test module
func TestMain(m *testing.M) {
	state := py.Initialize()

	ret := m.Run()

	python.PyEval_RestoreThread(state)
	python.Finalize()

	os.Exit(ret)
}

func TestGetPayload(t *testing.T) {
	pl := GetPayload("testhostname")
	assert.NotNil(t, pl)
}

func TestExternalHostTags(t *testing.T) {
	host := "localhost"
	eTags := externalhost.ExternalTags{"vsphere": []string{"foo", "bar"}}
	externalhost.AddExternalTags(host, eTags)
	externalhost.AddExternalTags(host+"2", eTags)

	pl := GetPayload(host)
	hpl := pl.ExternalHostPayload.Payload
	assert.Len(t, hpl, 2)
	assert.Equal(t, host, hpl[0][0])
	tags, ok := hpl[0][1].(externalhost.ExternalTags)
	require.True(t, ok)
	assert.Len(t, tags["vsphere"], 2)
}
