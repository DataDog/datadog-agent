// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/protocol"
)

func status(code uint32) error { return &protocol.ResponseError{Code: code} }

// pathErr wraps err the way the library's Share methods do.
func pathErr(err error) error { return &os.PathError{Op: "open", Path: `app\x.log`, Err: err} }

func TestClassify(t *testing.T) {
	dialRefused := &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}

	for _, tc := range []struct {
		name string
		err  error
		want ErrorKind
	}{
		{"nil", nil, ErrOther},
		{"plain error", errors.New("boom"), ErrOther},
		{"closed client", ErrClosed, ErrOther},
		{"wrapped closed client", fmt.Errorf("list: %w", ErrClosed), ErrOther},
		{"unknown status", status(0xC0000001), ErrOther}, // STATUS_UNSUCCESSFUL
		{"end of file is not an error kind", status(statusEndOfFile), ErrOther},
		{"not a directory", status(0xC0000103), ErrOther},

		// Session and connection failures.
		{"session expired", status(statusNetworkSessionExpired), ErrTransient},
		{"user session deleted", status(statusUserSessionDeleted), ErrTransient},
		{"connection disconnected", status(statusConnectionDisconnected), ErrTransient},
		{"network name deleted", pathErr(status(statusNetworkNameDeleted)), ErrTransient},
		{"connection reset status", status(statusConnectionReset), ErrTransient},
		{"io timeout status", status(statusIOTimeout), ErrTransient},
		{"insufficient server resources", status(statusInsuffServerResources), ErrTransient},
		{"invalid handle", status(statusInvalidHandle), ErrTransient},
		{"file closed", status(statusFileClosed), ErrTransient},
		{"transport error", &protocol.TransportError{Err: io.EOF}, ErrTransient},
		{"transport error after close", pathErr(&protocol.TransportError{Err: net.ErrClosed}), ErrTransient},
		{"invalid response", &protocol.InvalidResponseError{Message: "broken"}, ErrTransient},
		{"dial refused", fmt.Errorf("smb: connect to smb://h/s: %w", dialRefused), ErrTransient},
		{"dns failure", &net.DNSError{Err: "no such host", Name: "nope.invalid", IsNotFound: true}, ErrTransient},
		{"net closed", net.ErrClosed, ErrTransient},
		{"io eof", io.EOF, ErrTransient},
		{"unexpected eof", io.ErrUnexpectedEOF, ErrTransient},
		{"deadline", context.DeadlineExceeded, ErrTransient},
		{"canceled", context.Canceled, ErrTransient},
		{"os deadline", os.ErrDeadlineExceeded, ErrTransient},
		{"raw econnreset", syscall.ECONNRESET, ErrTransient},
		{"raw epipe", fmt.Errorf("write: %w", syscall.EPIPE), ErrTransient},

		// Missing files and directories.
		{"object name not found", pathErr(status(statusObjectNameNotFound)), ErrNotFound},
		{"object path not found", status(statusObjectPathNotFound), ErrNotFound},
		{"delete pending", status(statusDeletePending), ErrNotFound},
		{"file deleted", status(statusFileDeleted), ErrNotFound},
		{"no such file", status(statusNoSuchFile), ErrNotFound},
		{"os not exist", fmt.Errorf("x: %w", os.ErrNotExist), ErrNotFound},
		{"raw errno not exist", &os.PathError{Op: "open", Path: "x", Err: syscall.ENOENT}, ErrNotFound},

		// Failures that need user action.
		{"access denied", pathErr(status(statusAccessDenied)), ErrAuth},
		{"logon failure", status(statusLogonFailure), ErrAuth},
		{"wrong password", status(statusWrongPassword), ErrAuth},
		{"password expired", status(statusPasswordExpired), ErrAuth},
		{"account disabled", status(statusAccountDisabled), ErrAuth},
		{"account locked out", status(statusAccountLockedOut), ErrAuth},
		{"network access denied", status(statusNetworkAccessDenied), ErrAuth},
		{"bad share name", &os.PathError{Op: "mount", Path: `\\h\nope`, Err: status(statusBadNetworkName)}, ErrAuth},
		{"os permission", os.ErrPermission, ErrAuth},

		// Conflicting opens.
		{"sharing violation", pathErr(status(statusSharingViolation)), ErrSharing},
		{"lock conflict", status(statusFileLockConflict), ErrSharing},

		// A server status wins over the wrapping.
		{"status inside fmt wrapping", fmt.Errorf("read: %w", pathErr(status(statusSharingViolation))), ErrSharing},
		{"status inside backoff error", &backoffError{err: status(statusLogonFailure)}, ErrAuth},
		{"transient inside backoff error", &backoffError{err: dialRefused}, ErrTransient},
		{"redacted error keeps its kind", &redactedError{msg: "x", kind: ErrSharing}, ErrSharing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Classify(tc.err), "%v", tc.err)
		})
	}
}

func TestClassifyCompound(t *testing.T) {
	// A failed CREATE fails the rest of a related compound: the CREATE's
	// status decides.
	createFailed := &protocol.CompoundResponseError{Errors: []error{
		status(statusObjectNameNotFound),
		status(statusInvalidHandle),
		status(statusInvalidHandle),
	}}
	assert.Equal(t, ErrNotFound, Classify(createFailed))
	assert.Equal(t, ErrNotFound, Classify(pathErr(createFailed)))

	readFailed := &protocol.CompoundResponseError{Errors: []error{nil, status(statusFileLockConflict), nil}}
	assert.Equal(t, ErrSharing, Classify(readFailed))
}

func TestErrorKindString(t *testing.T) {
	for kind, want := range map[ErrorKind]string{
		ErrOther:      "other",
		ErrTransient:  "transient",
		ErrNotFound:   "not found",
		ErrAuth:       "auth",
		ErrSharing:    "sharing",
		ErrorKind(42): "other",
	} {
		assert.Equal(t, want, kind.String())
	}
}
