// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenRegularRejectsDescriptorPortal(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "descriptor-target")
	require.NoError(t, err)
	defer file.Close()

	sb, _, err := New([]string{"/dev"})
	require.NoError(t, err)
	defer sb.Close()

	handle, err := sb.OpenRegular(fmt.Sprintf("/dev/fd/%d", file.Fd()), "/")
	assert.Nil(t, handle)
	assert.ErrorContains(t, err, "file identity changed while opening")
}
