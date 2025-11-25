// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package containers

import (
	"errors"
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

// errorReader is a fake reader that returns a specific error
type errorReader struct {
	err error
}

func (er *errorReader) Read(_ []byte) (n int, err error) {
	return 0, er.err
}
func TestStripResolvConfReaderError(t *testing.T) {
	customErr := errors.New("custom read error")
	reader := &errorReader{err: customErr}

	rs := makeResolvStripper(resolvConfInputMaxSizeBytes)
	_, err := rs.stripResolvConf(100, reader)
	require.Error(t, err)
	require.ErrorIs(t, err, customErr)
}

func TestStripResolvConfTooBigInput(t *testing.T) {
	resolvConf := strings.Repeat("a", 5000)
	reader := strings.NewReader(resolvConf)

	rs := makeResolvStripper(resolvConfInputMaxSizeBytes)
	stripped, err := rs.stripResolvConf(len(resolvConf), reader)
	require.NoError(t, err)

	require.Equal(t, "<too big: kind=input size=5000>", stripped)
}
func TestStripResolvConfTooBigOutput(t *testing.T) {
	resolvConf := strings.Repeat("a", 2000)
	reader := strings.NewReader(resolvConf)

	rs := makeResolvStripper(resolvConfInputMaxSizeBytes)
	stripped, err := rs.stripResolvConf(len(resolvConf), reader)
	require.NoError(t, err)

	require.Equal(t, "<too big: kind=output size=2000>", stripped)
}
