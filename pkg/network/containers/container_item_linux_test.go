// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package containers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStripResolvConf(t *testing.T) {
	resolvConf := `
; comment goes here
# other comment goes here
nameserver 8.8.8.8
	# indented comment with spaces
	nameserver 8.8.4.4  
`
	reader := strings.NewReader(resolvConf)

	rs := makeResolvStripper(resolvConfInputMaxSizeBytes)
	stripped, err := rs.stripResolvConf(len(resolvConf), reader)
	require.NoError(t, err)

	require.Equal(t, "nameserver 8.8.8.8\nnameserver 8.8.4.4", stripped)
}
