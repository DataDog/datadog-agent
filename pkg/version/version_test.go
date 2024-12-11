// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	// full fledge
	v, err := New("1.2.3-pre+☢", "deadbeef")
	assert.NoError(t, err)
	assert.Equal(t, int64(1), v.Major)
	assert.Equal(t, int64(2), v.Minor)
	assert.Equal(t, int64(3), v.Patch)
	assert.Equal(t, "pre", v.Pre)
	assert.Equal(t, "☢", v.Meta)
	assert.Equal(t, "deadbeef", v.Commit)

	// only pre
	v, err = New("1.2.3-pre-pre.1", "")
	assert.NoError(t, err)
	assert.Equal(t, "pre-pre.1", v.Pre)

	// only meta
	v, err = New("1.2.3+☢.1+", "")
	assert.NoError(t, err)
	assert.Equal(t, "☢.1+", v.Meta)

	_, err = New("", "")
	assert.NotNil(t, err)
	_, err = New("1.2.", "")
	assert.NotNil(t, err)
	_, err = New("1.2.3.4", "")
	assert.NotNil(t, err)
	_, err = New("1.2.foo", "")
	assert.NotNil(t, err)
}

func TestString(t *testing.T) {
	v, _ := New("1.2.3-pre+☢", "123beef")
	assert.Equal(t, "1.2.3-pre+☢.commit.123beef", v.String())
}

func TestGetNumber(t *testing.T) {
	v, _ := New("1.2.3-pre+☢", "123beef")
	assert.Equal(t, "1.2.3", v.GetNumber())
}
