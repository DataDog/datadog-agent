// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableevents

/*
#cgo LDFLAGS: -framework CoreFoundation -framework IOKit

#include <stdlib.h>
#include "shutdown_cause_iokit_darwin.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const (
	// pmuBootFaultProperty is the IORegistry property carrying the previous
	// boot's PMU fault tokens. It is the sole source of shutdown-cause
	// classification input and payload.
	pmuBootFaultProperty = "IOPMUBootFaultInfo"

	// pmuTokenSeparator matches the flattening performed by the C reader.
	pmuTokenSeparator = "\x1f"

	// pmuBootFaultInitialBufferSize is
	// DD_PMU_MAX_TOKENS_PER_SERVICE * DD_PMU_MAX_TOKEN_CHARS from
	// shutdown_cause_iokit_darwin.c: the largest single service the C reader
	// will ever hand back. A service beyond either bound is dropped and
	// logged by the C reader itself, so there is nothing here to retry.
	pmuBootFaultInitialBufferSize = 88 * 53
)

// errShutdownCauseUnsupported reports a platform where IOPMUBootFaultInfo
// cannot exist. Intel Macs need an entirely different source, which is not
// implemented.
var errShutdownCauseUnsupported = errors.New("shutdown cause reporting requires arm64 Darwin")

// pmuBootFaultInfo is one boot's PMU fault payload, as read from the
// IORegistry. Tokens is the deduplicated union across every publishing PMU:
// several PMUs on the same machine tend to republish the same fault tokens,
// so which service reported a token is not tracked.
type pmuBootFaultInfo struct {
	Tokens []string
}

// readPMUBootFaultInfo reads IOPMUBootFaultInfo from every service in the
// IOService plane that publishes it. An absent property is not an error: it
// yields an empty result, which classifies as no event.
func readPMUBootFaultInfo() (pmuBootFaultInfo, error) {
	if runtime.GOARCH != "arm64" {
		return pmuBootFaultInfo{}, errShutdownCauseUnsupported
	}
	return readPMUBootFaultInfoWithBufferSize(pmuBootFaultInitialBufferSize)
}

// readPMUBootFaultInfoWithBufferSize performs the native read into a
// fixed-size buffer. The C reader drops and logs any service that would not
// fit rather than reporting the read as incomplete, so a non-zero status here
// is always a fatal IOKit failure.
func readPMUBootFaultInfoWithBufferSize(size int) (pmuBootFaultInfo, error) {
	buffer := C.malloc(C.size_t(size))
	if buffer == nil {
		return pmuBootFaultInfo{}, errors.New("failed to allocate PMU boot fault buffer")
	}
	defer C.free(buffer)

	var written C.size_t
	status := C.dd_pkg_notableevents_read_pmu_boot_fault_info(
		(*C.char)(buffer),
		C.size_t(size),
		&written)
	if status != 0 {
		return pmuBootFaultInfo{}, fmt.Errorf("failed to read %s from the IORegistry", pmuBootFaultProperty)
	}

	if written == 0 {
		return pmuBootFaultInfo{}, nil
	}
	payload := C.GoStringN((*C.char)(buffer), C.int(written))
	return parsePMUBootFaultPayload(payload), nil
}

// parsePMUBootFaultPayload splits the flattened native payload into its
// tokens and dedupes them. It performs no validation; the classifier owns
// that.
func parsePMUBootFaultPayload(payload string) pmuBootFaultInfo {
	var info pmuBootFaultInfo
	seen := make(map[string]struct{})
	for _, token := range strings.Split(payload, pmuTokenSeparator) {
		if token == "" {
			continue
		}
		if _, duplicate := seen[token]; duplicate {
			continue
		}
		seen[token] = struct{}{}
		info.Tokens = append(info.Tokens, token)
	}
	return info
}

// readBootSessionUUID reads kern.bootsessionuuid, which changes on every boot
// and is the dedup key for shutdown-cause events.
func readBootSessionUUID() (string, error) {
	value, err := unix.Sysctl("kern.bootsessionuuid")
	if err != nil {
		return "", fmt.Errorf("failed to read kern.bootsessionuuid: %w", err)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("kern.bootsessionuuid is empty")
	}
	return value, nil
}

// readBootTime reads kern.boottime as the event timestamp. The exact shutdown
// instant is not recoverable, so boot time is the tightest known upper bound.
func readBootTime() (time.Time, error) {
	value, err := unix.SysctlTimeval("kern.boottime")
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to read kern.boottime: %w", err)
	}
	if value.Sec <= 0 {
		return time.Time{}, errors.New("kern.boottime is not set")
	}
	return time.Unix(value.Sec, int64(value.Usec)*int64(time.Microsecond)).UTC(), nil
}
