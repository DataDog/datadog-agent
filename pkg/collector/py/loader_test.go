// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build cpython

// NOTICE: See TestMain function in `utils_test.go` for Python initialization
package py

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	l, _ := NewPythonCheckLoader()
	config := check.Config{Name: "testcheck"}
	config.Instances = append(config.Instances, []byte("foo: bar"))
	config.Instances = append(config.Instances, []byte("bar: baz"))

	instances, err := l.Load(config)
	require.Nil(t, err)
	assert.Equal(t, 2, len(instances))

	// the python module doesn't exist
	config = check.Config{Name: "doesntexist"}
	instances, err = l.Load(config)
	assert.NotNil(t, err)
	assert.Zero(t, len(instances))

	// the python module contains errors
	config = check.Config{Name: "bad"}
	instances, err = l.Load(config)
	assert.NotNil(t, err)
	assert.Zero(t, len(instances))

	// the python module is good but nothing derives from AgentCheck
	config = check.Config{Name: "foo"}
	instances, err = l.Load(config)
	assert.NotNil(t, err)
	assert.Zero(t, len(instances))
}

func TestNewPythonCheckLoader(t *testing.T) {
	loader, err := NewPythonCheckLoader()
	assert.Nil(t, err)
	assert.NotNil(t, loader)
}
