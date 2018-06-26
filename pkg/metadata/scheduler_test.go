// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package metadata

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/py"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
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

func TestNewScheduler(t *testing.T) {
	fwd := forwarder.NewDefaultForwarder(nil)
	fwd.Start()
	s := serializer.NewSerializer(fwd)
	c := NewScheduler(s, "hostname")
	assert.Equal(t, fwd, c.srl.Forwarder)
	assert.Equal(t, "hostname", c.hostname)
}
