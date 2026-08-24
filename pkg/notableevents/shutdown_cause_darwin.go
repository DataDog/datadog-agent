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
	pmuBootFaultProperty          = "IOPMUBootFaultInfo"
	pmuTokenSeparator             = "\x1f"
	pmuBootFaultInitialBufferSize = C.DD_PMU_MAX_TOKENS_PER_SERVICE * C.DD_PMU_MAX_TOKEN_CHARS
	maxShutdownTokens             = C.DD_PMU_MAX_TOKENS_PER_SERVICE
	maxShutdownTokenBytes         = C.DD_PMU_MAX_TOKEN_CHARS - 1
)

var errShutdownCauseUnsupported = errors.New("shutdown cause reporting requires arm64 Darwin")

// pmuBootFaultInfo is one boot's PMU fault payload, as read from the
// IORegistry. Tokens is the deduplicated union across every publishing PMU:
// several PMUs on the same machine tend to republish the same fault tokens,
// so which service reported a token is not tracked.
type pmuBootFaultInfo struct {
	Tokens []string
}

func readPMUBootFaultInfo() (pmuBootFaultInfo, error) {
	if runtime.GOARCH != "arm64" {
		return pmuBootFaultInfo{}, errShutdownCauseUnsupported
	}

	buffer := C.malloc(C.size_t(pmuBootFaultInitialBufferSize))
	if buffer == nil {
		return pmuBootFaultInfo{}, errors.New("failed to allocate PMU boot fault buffer")
	}
	defer C.free(buffer)

	var written C.size_t
	status := C.dd_pkg_notableevents_read_pmu_boot_fault_info(
		(*C.char)(buffer),
		C.size_t(pmuBootFaultInitialBufferSize),
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

func parsePMUBootFaultPayload(payload string) pmuBootFaultInfo {
	var info pmuBootFaultInfo
	for _, token := range strings.Split(payload, pmuTokenSeparator) {
		if token == "" {
			continue
		}
		info.Tokens = append(info.Tokens, token)
	}
	return info
}

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
