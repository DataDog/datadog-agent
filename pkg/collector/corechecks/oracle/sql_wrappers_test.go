// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"errors"
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsConnectionError(t *testing.T) {
	readTimeout := &net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}
	for _, tc := range []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil", nil, false},
		{"session killed", errors.New("ORA-00028: your session has been killed"), true},
		{"read timeout", readTimeout, true},
		{"wrapped read timeout", fmt.Errorf("failed to query v$dataguard_config %w SELECT 1", readTimeout), true},
		{"read timeout text", errors.New("read tcp 10.0.0.1:1->10.0.0.2:1522: i/o timeout"), true},
		{"unexpected TTC message", errors.New("TTC error: received code 16 during response reading"), true},
		{"invalid TTC field", errors.New("ORA-03146: invalid buffer length for TTC field"), true},
		{"open cursors exceeded", errors.New("ORA-01000: maximum open cursors exceeded"), true},
		{"missing table", errors.New("ORA-00942: table or view does not exist"), false},
		{"no rows", errors.New("sql: no rows in result set"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isConnectionError(tc.err))
		})
	}
}
