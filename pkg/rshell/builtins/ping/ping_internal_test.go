// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// White-box tests for unexported helpers in the ping package.
package ping

import (
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIsPermissionErrEPROTONOSUPPORT(t *testing.T) {
	assert.True(t, isPermissionErr(syscall.EPROTONOSUPPORT))
}

// ============================================================================
// isPermissionErr
// ============================================================================

func TestIsPermissionErrNil(t *testing.T) {
	assert.False(t, isPermissionErr(nil))
}

func TestIsPermissionErrEPERM(t *testing.T) {
	assert.True(t, isPermissionErr(syscall.EPERM))
}

func TestIsPermissionErrEACCES(t *testing.T) {
	assert.True(t, isPermissionErr(syscall.EACCES))
}

func TestIsPermissionErrWrappedEPERM(t *testing.T) {
	// net.OpError wrapping an os.SyscallError wrapping EPERM.
	inner := &net.OpError{
		Op:  "socket",
		Err: fmt.Errorf("wrapped: %w", syscall.EPERM),
	}
	assert.True(t, isPermissionErr(inner))
}

func TestIsPermissionErrWrappedEACCES(t *testing.T) {
	inner := &net.OpError{
		Op:  "socket",
		Err: fmt.Errorf("wrapped: %w", syscall.EACCES),
	}
	assert.True(t, isPermissionErr(inner))
}

func TestIsPermissionErrStringFallback(t *testing.T) {
	assert.True(t, isPermissionErr(errors.New("operation not permitted")))
	assert.True(t, isPermissionErr(errors.New("OPERATION NOT PERMITTED"))) // case-insensitive
	assert.True(t, isPermissionErr(errors.New("access is denied")))
	assert.True(t, isPermissionErr(errors.New("permission denied")))
	// Linux: EPROTONOSUPPORT wrapped as string (e.g. "listen udp4: socket: protocol not supported")
	assert.True(t, isPermissionErr(errors.New("protocol not supported")))
	// Windows WSAEPROTONOSUPPORT (10043): returned by pro-bing when unprivileged
	// raw socket creation fails; privileged retry should be attempted.
	assert.True(t, isPermissionErr(errors.New("The requested protocol has not been configured into the system, or no implementation for it exists.")))
}

func TestIsPermissionErrUnrelated(t *testing.T) {
	assert.False(t, isPermissionErr(errors.New("connection refused")))
	assert.False(t, isPermissionErr(errors.New("no such host")))
	assert.False(t, isPermissionErr(errors.New("i/o timeout")))
}

// ============================================================================
// parsePingDuration
// ============================================================================

func TestParsePingDurationGoDuration(t *testing.T) {
	d, err := parsePingDuration("1s")
	assert.NoError(t, err)
	assert.Equal(t, time.Second, d)

	d, err = parsePingDuration("500ms")
	assert.NoError(t, err)
	assert.Equal(t, 500*time.Millisecond, d)

	d, err = parsePingDuration("1m30s")
	assert.NoError(t, err)
	assert.Equal(t, 90*time.Second, d)
}

func TestParsePingDurationIntegerSeconds(t *testing.T) {
	d, err := parsePingDuration("1")
	assert.NoError(t, err)
	assert.Equal(t, time.Second, d)

	d, err = parsePingDuration("30")
	assert.NoError(t, err)
	assert.Equal(t, 30*time.Second, d)
}

func TestParsePingDurationFloatSeconds(t *testing.T) {
	d, err := parsePingDuration("0.2")
	assert.NoError(t, err)
	assert.Equal(t, 200*time.Millisecond, d)

	d, err = parsePingDuration("1.5")
	assert.NoError(t, err)
	assert.Equal(t, 1500*time.Millisecond, d)
}

func TestParsePingDurationInvalid(t *testing.T) {
	_, err := parsePingDuration("abc")
	assert.Error(t, err)

	_, err = parsePingDuration("1x")
	assert.Error(t, err)
}

func TestParsePingDurationNegative(t *testing.T) {
	_, err := parsePingDuration("-1")
	assert.Error(t, err)
}

func TestParsePingDurationInfNaN(t *testing.T) {
	_, err := parsePingDuration("inf")
	assert.Error(t, err, "inf should be rejected")

	_, err = parsePingDuration("+Inf")
	assert.Error(t, err, "+Inf should be rejected")

	_, err = parsePingDuration("NaN")
	assert.Error(t, err, "NaN should be rejected")
}

func TestParsePingDurationNegativeGoLiteral(t *testing.T) {
	// Negative Go duration literals (e.g. "-1s") are explicitly rejected.
	// Providing a negative wait/interval is clearly invalid; an error is
	// more useful than silently clamping to the minimum.
	_, err := parsePingDuration("-1s")
	assert.Error(t, err, "negative Go literal should be rejected")

	_, err = parsePingDuration("-250ms")
	assert.Error(t, err, "negative Go millisecond literal should be rejected")
}

func TestParsePingDurationOverflow(t *testing.T) {
	// Very large finite float overflows time.Duration (int64 ns) to negative.
	// Must be caught before the conversion.
	_, err := parsePingDuration("1e20")
	assert.Error(t, err, "1e20 seconds should be rejected as overflow")
}

// ============================================================================
// clampInt / clampDuration
// ============================================================================

func TestClampIntAtMin(t *testing.T) {
	assert.Equal(t, 1, clampInt(-5, 1, 20))
	assert.Equal(t, 1, clampInt(0, 1, 20))
	assert.Equal(t, 1, clampInt(1, 1, 20))
}

func TestClampIntAtMax(t *testing.T) {
	assert.Equal(t, 20, clampInt(21, 1, 20))
	assert.Equal(t, 20, clampInt(1<<30, 1, 20))
}

func TestClampIntMiddle(t *testing.T) {
	assert.Equal(t, 10, clampInt(10, 1, 20))
}

func TestClampDurationAtMin(t *testing.T) {
	assert.Equal(t, minInterval, clampDuration(0, minInterval, 60e9))
	assert.Equal(t, minInterval, clampDuration(1, minInterval, 60e9))
	assert.Equal(t, minInterval, clampDuration(minInterval, minInterval, 60e9))
}

func TestClampDurationAtMax(t *testing.T) {
	assert.Equal(t, maxWait, clampDuration(maxWait+1, 0, maxWait))
}

// ============================================================================
// durToMS
// ============================================================================

func TestDurToMS(t *testing.T) {
	assert.InDelta(t, 1.0, durToMS(1e6), 1e-9)        // 1ms
	assert.InDelta(t, 17.045, durToMS(17045e3), 1e-6) // 17.045ms
	assert.Equal(t, 0.0, durToMS(0))
}
