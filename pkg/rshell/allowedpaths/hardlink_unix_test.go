// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package allowedpaths

import (
	"io/fs"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type noSysFileInfo struct{}

func (noSysFileInfo) Name() string       { return "f" }
func (noSysFileInfo) Size() int64        { return 0 }
func (noSysFileInfo) Mode() fs.FileMode  { return 0 }
func (noSysFileInfo) ModTime() time.Time { return time.Time{} }
func (noSysFileInfo) IsDir() bool        { return false }
func (noSysFileInfo) Sys() any           { return nil }

// TestOpenFileLinkCountFailsClosedWhenUnknown pins that a missing link count
// is reported as an error rather than as "single-linked", so the write guard
// refuses the target instead of silently allowing it.
func TestOpenFileLinkCountFailsClosedWhenUnknown(t *testing.T) {
	_, err := openFileLinkCount(nil, noSysFileInfo{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errLinkCountUnavailable)
}
