// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package opener

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

func TestDirectFingerprintReadRequiresLinux(t *testing.T) {
	opener := NewFileOpener()
	flags := []types.FileOpenFlag{types.FileOpenFlagDirect}

	_, err := opener.ReadDirectFingerprintRange("/tmp/app.log", 16, flags)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Linux")
}
