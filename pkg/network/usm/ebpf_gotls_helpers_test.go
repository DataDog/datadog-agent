// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/go/binversion"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func TestExpectedGoTLSInspectionError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "not go executable",
			err:      fmt.Errorf("could not get Go toolchain version from ELF binary file: %w", binversion.ErrNotGoExe),
			expected: true,
		},
		{
			name:     "no symbol section",
			err:      fmt.Errorf("read symbols: %w", safeelf.ErrNoSymbols),
			expected: true,
		},
		{
			name:     "requested symbols not found",
			err:      fmt.Errorf("inspect symbols: %w", bininspect.ErrSymbolsNotFound),
			expected: true,
		},
		{
			name:     "unexpected inspection failure",
			err:      errors.New("malformed pclntab"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, isExpectedGoTLSInspectionError(tt.err))
		})
	}
}
