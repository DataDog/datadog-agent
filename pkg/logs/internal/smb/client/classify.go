// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/protocol"
)

// ErrorKind tells the launcher and tailers how to react to an error.
type ErrorKind int

const (
	// ErrOther is an error with no specific handling: report it and retry on
	// the next poll.
	ErrOther ErrorKind = iota
	// ErrTransient is a network or session failure. The session is replaced
	// (with backoff); offsets kept in memory stay valid.
	ErrTransient
	// ErrNotFound means the file or directory does not exist (or is being
	// deleted), typically because of a rotation.
	ErrNotFound
	// ErrAuth is a failure that needs user action: bad credentials, a
	// disabled or locked account, access denied, or an unknown share.
	ErrAuth
	// ErrSharing means another open conflicts with ours (sharing violation or
	// byte-range lock). Retry on the next poll.
	ErrSharing
	// ErrTooLarge means a directory has more entries, or bigger ones, than the
	// client lists (see ErrTooManyEntries, ErrListingTooLarge and
	// ErrNameTooLong). It needs a change of the source's path or of the
	// directory; reading it again would give the same result.
	ErrTooLarge
)

// String implements fmt.Stringer.
func (k ErrorKind) String() string {
	switch k {
	case ErrTransient:
		return "transient"
	case ErrNotFound:
		return "not found"
	case ErrAuth:
		return "auth"
	case ErrSharing:
		return "sharing"
	case ErrTooLarge:
		return "too large"
	default:
		return "other"
	}
}

// NTSTATUS codes ([MS-ERREF] 2.3.1). The vendored library keeps its table in
// an internal package, so the codes the source cares about are repeated here.
const (
	statusInvalidHandle          = 0xC0000008
	statusNoSuchFile             = 0xC000000F
	statusEndOfFile              = 0xC0000011
	statusAccessDenied           = 0xC0000022
	statusObjectNameNotFound     = 0xC0000034
	statusObjectPathNotFound     = 0xC000003A
	statusSharingViolation       = 0xC0000043
	statusFileLockConflict       = 0xC0000054
	statusDeletePending          = 0xC0000056
	statusNoSuchUser             = 0xC0000064
	statusWrongPassword          = 0xC000006A
	statusLogonFailure           = 0xC000006D
	statusAccountRestriction     = 0xC000006E
	statusPasswordExpired        = 0xC0000071
	statusAccountDisabled        = 0xC0000072
	statusIOTimeout              = 0xC00000B5
	statusNotSupported           = 0xC00000BB
	statusNetworkNameDeleted     = 0xC00000C9
	statusNetworkAccessDenied    = 0xC00000CA
	statusBadNetworkName         = 0xC00000CC
	statusRequestNotAccepted     = 0xC00000D0
	statusFileDeleted            = 0xC0000123
	statusFileClosed             = 0xC0000128
	statusLogonTypeNotGranted    = 0xC000015B
	statusAccountExpired         = 0xC0000193
	statusUserSessionDeleted     = 0xC0000203
	statusInsuffServerResources  = 0xC0000205
	statusConnectionDisconnected = 0xC000020C
	statusConnectionReset        = 0xC000020D
	statusPasswordMustChange     = 0xC0000224
	statusAccountLockedOut       = 0xC0000234
	statusConnectionAborted      = 0xC0000241
	statusNetworkSessionExpired  = 0xC000035C
)

// statusKinds maps server status codes to kinds. Codes not listed are ErrOther.
// STATUS_END_OF_FILE is not an error for this package: ReadAt turns it into
// an empty read.
var statusKinds = map[uint32]ErrorKind{
	// The session or connection is gone, or the server lost our handle.
	statusNetworkSessionExpired:  ErrTransient,
	statusUserSessionDeleted:     ErrTransient,
	statusConnectionDisconnected: ErrTransient,
	statusConnectionReset:        ErrTransient,
	statusConnectionAborted:      ErrTransient,
	statusNetworkNameDeleted:     ErrTransient,
	statusIOTimeout:              ErrTransient,
	statusInsuffServerResources:  ErrTransient,
	statusRequestNotAccepted:     ErrTransient,
	statusInvalidHandle:          ErrTransient,
	statusFileClosed:             ErrTransient,

	statusObjectNameNotFound: ErrNotFound,
	statusObjectPathNotFound: ErrNotFound,
	statusDeletePending:      ErrNotFound,
	statusFileDeleted:        ErrNotFound,
	statusNoSuchFile:         ErrNotFound,

	statusAccessDenied:        ErrAuth,
	statusNetworkAccessDenied: ErrAuth,
	statusLogonFailure:        ErrAuth,
	statusWrongPassword:       ErrAuth,
	statusNoSuchUser:          ErrAuth,
	statusAccountRestriction:  ErrAuth,
	statusPasswordExpired:     ErrAuth,
	statusPasswordMustChange:  ErrAuth,
	statusAccountDisabled:     ErrAuth,
	statusAccountExpired:      ErrAuth,
	statusAccountLockedOut:    ErrAuth,
	statusLogonTypeNotGranted: ErrAuth,
	statusBadNetworkName:      ErrAuth,

	statusSharingViolation: ErrSharing,
	statusFileLockConflict: ErrSharing,
}

// Classify reports how err should be handled. A status code sent by the
// server takes precedence over everything else in the chain; Classify(nil)
// is ErrOther.
func Classify(err error) ErrorKind {
	if err == nil {
		return ErrOther
	}
	// The Agent's own errors come first: they wrap library errors, redacted or
	// not, that classify otherwise.
	if errors.Is(err, errGuestSession) || errors.Is(err, ErrNotEncrypted) || errors.Is(err, ErrLogonStopped) {
		return ErrAuth
	}
	var redacted *redactedError
	if errors.As(err, &redacted) {
		return redacted.kind
	}
	if errors.Is(err, ErrClosed) {
		return ErrOther
	}
	if errors.Is(err, ErrTooManyEntries) || errors.Is(err, ErrListingTooLarge) || errors.Is(err, ErrNameTooLong) {
		return ErrTooLarge
	}
	if code, ok := statusCode(err); ok {
		return statusKinds[code]
	}

	var (
		transportErr *protocol.TransportError
		invalidErr   *protocol.InvalidResponseError
		opErr        *net.OpError
		dnsErr       *net.DNSError
	)
	switch {
	case errors.As(err, &transportErr),
		errors.As(err, &invalidErr),
		errors.As(err, &opErr),
		errors.As(err, &dnsErr),
		errors.Is(err, net.ErrClosed),
		errors.Is(err, io.EOF),
		errors.Is(err, io.ErrUnexpectedEOF),
		errors.Is(err, os.ErrDeadlineExceeded),
		errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, context.Canceled),
		errors.Is(err, syscall.ECONNRESET),
		errors.Is(err, syscall.ECONNREFUSED),
		errors.Is(err, syscall.ECONNABORTED),
		errors.Is(err, syscall.EPIPE):
		return ErrTransient
	case errors.Is(err, os.ErrNotExist):
		return ErrNotFound
	case errors.Is(err, os.ErrPermission):
		return ErrAuth
	}
	return ErrOther
}

// statusCode returns the first NTSTATUS code in err's chain. For a compound
// request that is the earliest failed operation, which is the one that
// caused the others to fail.
func statusCode(err error) (uint32, bool) {
	var redacted *redactedError
	if errors.As(err, &redacted) {
		return redacted.code, redacted.hasCode
	}
	var respErr *protocol.ResponseError
	if errors.As(err, &respErr) && respErr != nil {
		return respErr.Code, true
	}
	return 0, false
}
