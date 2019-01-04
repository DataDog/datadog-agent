// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython

// NOTICE: See TestMain function in `utils_test.go` for Python initialization
package py

import (
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	l, _ := NewPythonCheckLoader()
	config := integration.Config{Name: "testcheck"}
	config.Instances = append(config.Instances, []byte("foo: bar"))
	config.Instances = append(config.Instances, []byte("bar: baz"))

	instances, err := l.Load(config)
	require.Nil(t, err)
	assert.Equal(t, 2, len(instances))

	// the python module doesn't exist
	config = integration.Config{Name: "doesntexist"}
	instances, err = l.Load(config)
	assert.NotNil(t, err)
	assert.Zero(t, len(instances))

	// the python module contains errors
	config = integration.Config{Name: "bad"}
	instances, err = l.Load(config)
	assert.NotNil(t, err)
	assert.Zero(t, len(instances))

	// the python module works
	config = integration.Config{Name: "working"}
	config.Instances = append(config.Instances, []byte("foo: bar"))
	config.Instances = append(config.Instances, []byte("bar: baz"))
	instances, err = l.Load(config)
	require.Nil(t, err)
	assert.Equal(t, 2, len(instances))

	// the python module is good but nothing derives from AgentCheck
	config = integration.Config{Name: "foo"}
	instances, err = l.Load(config)
	assert.NotNil(t, err)
	assert.Zero(t, len(instances))
}

func TestLoadVersion(t *testing.T) {
	l, _ := NewPythonCheckLoader()

	// the python module has no version
	config := integration.Config{Name: "working"}
	config.Instances = append(config.Instances, []byte("foo: bar"))
	instances, err := l.Load(config)
	require.Nil(t, err)
	assert.Equal(t, 1, len(instances))

	// the python module has a version
	config = integration.Config{Name: "version"}
	config.Instances = append(config.Instances, []byte("foo: bar"))
	instances, err = l.Load(config)
	require.Nil(t, err)
	assert.Equal(t, 1, len(instances))
	assert.Equal(t, "1.0.0", instances[0].Version())
}

// Some bugs can appear only if python use the same OS thread for several actions. We lock there to be sure to test this case
func TestLoadVersionLock(t *testing.T) {
	runtime.LockOSThread()
	TestLoadVersion(t)
	runtime.UnlockOSThread()
}

func TestLoadandRun(t *testing.T) {
	l, _ := NewPythonCheckLoader()

	// the python module loads fine but check fails
	config := integration.Config{Name: "version"}
	config.Instances = append(config.Instances, []byte("foo: bar"))
	instances, err := l.Load(config)
	require.Nil(t, err)
	assert.Equal(t, 1, len(instances))
	err = instances[0].Run()
	assert.NotNil(t, err)

	// the python module loads and check runs
	config = integration.Config{Name: "working"}
	config.Instances = append(config.Instances, []byte("foo: bar"))
	instances, err = l.Load(config)
	require.Nil(t, err)
	assert.Equal(t, 1, len(instances))
	err = instances[0].Run()
	assert.Nil(t, err)

}

func TestLoadandRunLock(t *testing.T) {
	runtime.LockOSThread()
	TestLoadandRun(t)
	runtime.UnlockOSThread()
}

func TestNewPythonCheckLoader(t *testing.T) {
	loader, err := NewPythonCheckLoader()
	assert.Nil(t, err)
	assert.NotNil(t, loader)
}
