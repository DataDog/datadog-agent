// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython

package v5

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/py"
	python "github.com/DataDog/go-python3"
	"github.com/stretchr/testify/assert"
)

// Setup the test module
func TestMain(m *testing.M) {
	state := py.Initialize()

	ret := m.Run()

	python.PyEval_RestoreThread(state)
	python.Py_Finalize()

	os.Exit(ret)
}

func TestGetPayload(t *testing.T) {
	pl := GetPayload("testhostname")
	assert.NotNil(t, pl)
}
