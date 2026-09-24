// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package msi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentMSIName(t *testing.T) {
	assert.Equal(t, "datadog-agent-7.85.0-1-x86_64.msi", AgentMSIName("7.85.0-1", false))
	assert.Equal(t, "datadog-fips-agent-7.85.0-1-x86_64.msi", AgentMSIName("7.85.0-1", true))
	assert.Equal(t, "datadog-fips-agent-7.85.0-devel.git.123-1-x86_64.msi", AgentMSIName("7.85.0-devel.git.123-1", true))
}

func TestAgentProductName(t *testing.T) {
	assert.Equal(t, "Datadog Agent", AgentProductName(false))
	assert.Equal(t, "Datadog FIPS Agent", AgentProductName(true))
}

func TestFindAgentMSI(t *testing.T) {
	const base = "datadog-agent-7.85.0-1-x86_64.msi"
	const fips = "datadog-fips-agent-7.85.0-1-x86_64.msi"
	tests := []struct {
		name     string
		fipsMode bool
		files    []string
		want     string
		wantErr  string
	}{
		{name: "base", files: []string{base}, want: base},
		{name: "fips", fipsMode: true, files: []string{fips}, want: fips},
		{name: "base ignores fips", files: []string{base, fips}, want: base},
		{name: "fips ignores base", fipsMode: true, files: []string{base, fips}, want: fips},
		{name: "no fips fallback", fipsMode: true, files: []string{base}, wantErr: "no MSIs in package"},
		{name: "no base fallback", files: []string{fips}, wantErr: "no MSIs in package"},
		{name: "missing", wantErr: "no MSIs in package"},
		{name: "ambiguous fips", fipsMode: true, files: []string{fips, "datadog-fips-agent-7.85.1-1-x86_64.msi"}, wantErr: "too many MSIs in package"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range tt.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0600))
			}
			got, err := FindAgentMSI(dir, tt.fipsMode)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(dir, tt.want), got)
		})
	}
}
