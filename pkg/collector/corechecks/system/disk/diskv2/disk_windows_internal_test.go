// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package diskv2

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeWindowsDeviceName(t *testing.T) {
	assert.Equal(t, "c:", normalizeWindowsDeviceName(`C:\`))
	assert.Equal(t, "f:/tlog", normalizeWindowsDeviceName(`F:\Tlog`))
	assert.Equal(t, `?\volume{123}`, normalizeWindowsDeviceName(`\\?\Volume{123}\`))
}
