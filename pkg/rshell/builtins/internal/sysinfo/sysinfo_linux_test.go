// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package sysinfo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeProc(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0600))
}

func TestReadUptimeSeconds(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    float64
		wantErr bool
	}{
		{"typical", "93784.12 456789.34\n", 93784.12, false},
		{"integer only", "3600 1800\n", 3600, false},
		{"empty file", "", 0, true},
		{"non-numeric first field", "abc 123\n", 0, true},
		{"missing second field is ok", "12345.6\n", 12345.6, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeProc(t, dir, "uptime", tt.content)
			orig := uptimePath
			uptimePath = filepath.Join(dir, "uptime")
			t.Cleanup(func() { uptimePath = orig })

			got, err := readUptimeSeconds()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.InDelta(t, tt.want, got, 0.001)
			}
		})
	}
}

func TestReadLoadAvg(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		want1, want5 float64
		want15       float64
		wantErr      bool
	}{
		{"typical", "1.23 4.56 7.89 2/100 12345\n", 1.23, 4.56, 7.89, false},
		{"empty file", "", 0, 0, 0, true},
		{"fewer than 3 fields", "1.0 2.0\n", 0, 0, 0, true},
		{"non-numeric field", "1.0 bad 3.0 2/100 1\n", 0, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeProc(t, dir, "loadavg", tt.content)
			orig := loadavgPath
			loadavgPath = filepath.Join(dir, "loadavg")
			t.Cleanup(func() { loadavgPath = orig })

			l1, l5, l15, err := readLoadAvg()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.InDelta(t, tt.want1, l1, 0.001)
				assert.InDelta(t, tt.want5, l5, 0.001)
				assert.InDelta(t, tt.want15, l15, 0.001)
			}
		})
	}
}
