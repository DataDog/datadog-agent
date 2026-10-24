// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package etw

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormattingOutTypeFormatsStatusCodesAsHex(t *testing.T) {
	for _, outType := range []uintptr{tdhOutTypeWin32Error, tdhOutTypeNTStatus, tdhOutTypeHResult} {
		assert.Equal(t, uintptr(tdhOutTypeHexInt32), formattingOutType(outType))
	}
	// Other output types, such as TDH_OUTTYPE_STRING and TDH_OUTTYPE_UNSIGNEDINT, are left as is.
	for _, outType := range []uintptr{1, 8} {
		assert.Equal(t, outType, formattingOutType(outType))
	}
}
