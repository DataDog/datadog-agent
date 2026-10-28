// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package privilegedhelper

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/allowedpaths"
)

// Two read-only policies must not intersect into a write grant, even when the
// selected directory is literally named with an access suffix.
func TestReadOnlyIntersectionOfSuffixedDirectoryStaysReadOnly(t *testing.T) {
	parent := t.TempDir()
	tenant := filepath.Join(parent, "tenant")
	require.NoError(t, os.Mkdir(tenant, 0755))
	require.NoError(t, os.Mkdir(tenant+":rw", 0755))
	file := filepath.Join(tenant, "file")
	require.NoError(t, os.WriteFile(file, []byte("protected"), 0644))

	credential, private := testCredential(t)
	credential.AllowedPaths = []string{parent + ":ro"}
	verified, err := credential.Verify(signedRequest(t, private, func(task *PrivateActionTask) {
		task.GetSystemInputs().GetRemoteAction().AllowedPaths = []string{tenant + ":rw:ro"}
	}), time.Now())
	require.NoError(t, err)
	require.Equal(t, []string{tenant + ":rw:ro"}, verified.AllowedPaths)

	sb, _, err := allowedpaths.New(verified.AllowedPaths)
	require.NoError(t, err)
	defer sb.Close()

	f, err := sb.Open(file, parent, os.O_WRONLY|os.O_TRUNC, 0)
	if err == nil {
		f.Close()
	}
	assert.Error(t, err)

	got, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "protected", string(got))
}
