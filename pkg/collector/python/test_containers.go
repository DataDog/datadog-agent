// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build python,test

package python

import (
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/stretchr/testify/assert"
)

import "C"

func testIsContainerExcluded(t *testing.T) {
	filter = &containers.Filter{
		Enabled: true,
	}
	defer func() { filter = nil }()

	r, err := regexp.Compile("bar")
	assert.Nil(t, err)
	filter.ImageBlacklist = append(filter.ImageBlacklist, r)

	r, err = regexp.Compile("white")
	assert.Nil(t, err)
	filter.NamespaceWhitelist = append(filter.NamespaceWhitelist, r)

	r, err = regexp.Compile("black")
	assert.Nil(t, err)
	filter.NamespaceBlacklist = append(filter.NamespaceBlacklist, r)

	assert.Equal(t, IsContainerExcluded(C.CString("foo"), C.CString("bar"), C.CString("ns")), C.int(1))
	assert.Equal(t, IsContainerExcluded(C.CString("foo"), C.CString("bar"), C.CString("white")), C.int(0))
	assert.Equal(t, IsContainerExcluded(C.CString("foo"), C.CString("baz"), C.CString("black")), C.int(1))
	assert.Equal(t, IsContainerExcluded(C.CString("foo"), C.CString("baz"), nil), C.int(0))
}
