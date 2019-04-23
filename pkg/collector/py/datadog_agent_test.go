// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython

package py

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metadata/externalhost"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	python "github.com/sbinet/go-python"
)

func TestAddExternalTagsBindings(t *testing.T) {
	gstate := newStickyLock()
	defer gstate.unlock()

	module := python.PyImport_ImportModule("external_host_tags")
	require.NotNil(t, module)
	f := module.GetAttrString("test")
	require.NotNil(t, f)
	// this will add 1 entry to the external host metadata cache
	f.Call(python.PyList_New(0), python.PyDict_New())

	ehp := *(externalhost.GetPayload())
	require.Len(t, ehp, 1)
	tuple := ehp[0]
	require.Len(t, tuple, 2)
	assert.Contains(t, "test-py-localhost", tuple[0])
	eTags := externalhost.ExternalTags{"test-source-type": []string{"tag1", "tag2", "tag3"}}
	assert.Equal(t, tuple[1], eTags)
}
