// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package process

import (
	"fmt"
	"io"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/golabels"
	"github.com/DataDog/datadog-agent/pkg/security/otelprocessctx"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/golabelsctx"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/otelattrs"
)

func TestClassifySpanCtxError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want spanCtxStatus
	}{
		{
			name: "nil",
			err:  nil,
			want: spanCtxOK,
		},
		{
			name: "eacces",
			err:  syscall.EACCES,
			want: spanCtxUnreadable,
		},
		{
			name: "eperm",
			err:  syscall.EPERM,
			want: spanCtxUnreadable,
		},
		{
			name: "os ErrPermission",
			err:  os.ErrPermission,
			want: spanCtxUnreadable,
		},
		{
			name: "eacces wrapped with errSpanCtxMalformed",
			err:  fmt.Errorf("%w: open /host/proc/123/root/app/ddtrace.so: %w", errSpanCtxMalformed, &os.PathError{Op: "open", Path: "/host/proc/123/root/app/ddtrace.so", Err: syscall.EACCES}),
			want: spanCtxUnreadable,
		},
		{
			name: "os ErrPermission wrapped with golabels ErrMalformed",
			err:  fmt.Errorf("%w: %w", golabels.ErrMalformed, os.ErrPermission),
			want: spanCtxUnreadable,
		},
		{
			name: "enoent wrapped with errSpanCtxMalformed",
			err:  fmt.Errorf("%w: %w", errSpanCtxMalformed, syscall.ENOENT),
			want: spanCtxGone,
		},
		{
			name: "eof wrapped with otelprocessctx ErrMalformed",
			err:  fmt.Errorf("%w: %w", otelprocessctx.ErrMalformed, io.EOF),
			want: spanCtxGone,
		},
		{
			name: "unpublished",
			err:  otelprocessctx.ErrUnpublished,
			want: spanCtxUnpublished,
		},
		{
			name: "legacy version",
			err:  otelprocessctx.ErrLegacyVersion,
			want: spanCtxNotApplicable,
		},
		{
			name: "malformed",
			err:  fmt.Errorf("%w: bad signature", otelprocessctx.ErrMalformed),
			want: spanCtxMalformed,
		},
		{
			name: "golabels map lookup",
			err:  golabelsctx.ErrMapLookup,
			want: spanCtxMapError,
		},
		{
			name: "otelattrs stale id",
			err:  otelattrs.ErrStaleID,
			want: spanCtxStaleID,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, classifySpanCtxError(tc.err))
		})
	}
}
