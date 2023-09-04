// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetNumberOfWarnings(t *testing.T) {
	setupTest(t)

	assert.Equal(t, 0, GetNumberOfWarnings())

	noProxyIgnoredWarningMap["test url"] = true
	assert.Equal(t, 1, GetNumberOfWarnings())

	noProxyUsedInFuture["test url"] = true
	assert.Equal(t, 2, GetNumberOfWarnings())

	noProxyChanged["test url"] = true
	assert.Equal(t, 3, GetNumberOfWarnings())
}

func TestGetProxyIgnoredWarnings(t *testing.T) {
	setupTest(t)

	noProxyIgnoredWarningMap["test url 1"] = true
	noProxyIgnoredWarningMap["test url 2"] = true

	warn := GetProxyIgnoredWarnings()
	sort.Strings(warn)

	assert.Equal(t, []string{"test url 1", "test url 2"}, warn)
}

func TestGetProxyUsedInFutureWarnings(t *testing.T) {
	setupTest(t)

	noProxyUsedInFuture["test url 1"] = true
	noProxyUsedInFuture["test url 2"] = true

	warn := GetProxyUsedInFutureWarnings()
	sort.Strings(warn)

	assert.Equal(t, []string{"test url 1", "test url 2"}, warn)
}

func TestGetProxyChangedWarnings(t *testing.T) {
	setupTest(t)

	noProxyChanged["test url 1"] = true
	noProxyChanged["test url 2"] = true

	warn := GetProxyChangedWarnings()
	sort.Strings(warn)

	assert.Equal(t, []string{"test url 1", "test url 2"}, warn)
}
