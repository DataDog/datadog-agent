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

	// pmuTokenSeparator and pmuServiceSeparator match the flattening performed
	// by the C reader.
	pmuTokenSeparator   = "\x1f"
	pmuServiceSeparator = "\x1e"

	// pmuBootFaultInitialBufferSize holds every payload observed in testing
	// with room to spare; pmuBootFaultMaxBufferSize is the single retry.
	pmuBootFaultInitialBufferSize = 16 * 1024
	pmuBootFaultMaxBufferSize     = 64 * 1024
)

// errShutdownCauseUnsupported reports a platform where IOPMUBootFaultInfo
// cannot exist. Intel Macs need an entirely different source, which is not
// implemented.
var errShutdownCauseUnsupported = errors.New("shutdown cause reporting requires arm64 Darwin")

// pmuBootFaultInfo is one boot's PMU fault payload, as read from the
// IORegistry. Groups holds one entry per service that published the property;
// a machine with several PMUs reports several groups.
type pmuBootFaultInfo struct {
	Groups [][]string
}

// readPMUBootFaultInfo reads IOPMUBootFaultInfo from every service in the
// IOService plane that publishes it. An absent property is not an error: it
// yields an empty result, which classifies as no event.
func readPMUBootFaultInfo() (pmuBootFaultInfo, error) {
	if runtime.GOARCH != "arm64" {
		return pmuBootFaultInfo{}, errShutdownCauseUnsupported
	}

	for _, size := range []int{pmuBootFaultInitialBufferSize, pmuBootFaultMaxBufferSize} {
		info, tooSmall, err := readPMUBootFaultInfoWithBufferSize(size)
		if err != nil {
			return pmuBootFaultInfo{}, err
		}
		if !tooSmall {
			return info, nil
		}
	}
	return pmuBootFaultInfo{}, fmt.Errorf("%s exceeds %d bytes", pmuBootFaultProperty, pmuBootFaultMaxBufferSize)
}

// readPMUBootFaultInfoWithBufferSize performs one attempt at the native read,
// reporting separately whether the buffer was too small so the caller can
// retry larger.
func readPMUBootFaultInfoWithBufferSize(size int) (pmuBootFaultInfo, bool, error) {
	buffer := C.malloc(C.size_t(size))
	if buffer == nil {
		return pmuBootFaultInfo{}, false, errors.New("failed to allocate PMU boot fault buffer")
	}
	defer C.free(buffer)

	var written, services C.size_t
	status := C.dd_pkg_notableevents_read_pmu_boot_fault_info(
		(*C.char)(buffer),
		C.size_t(size),
		&written,
		&services)

	switch status {
	case 0:
	case -2:
		return pmuBootFaultInfo{}, true, nil
	default:
		return pmuBootFaultInfo{}, false, fmt.Errorf("failed to read %s from the IORegistry", pmuBootFaultProperty)
	}

	if services == 0 || written == 0 {
		return pmuBootFaultInfo{}, false, nil
	}
	payload := C.GoStringN((*C.char)(buffer), C.int(written))
	return parsePMUBootFaultPayload(payload), false, nil
}

// parsePMUBootFaultPayload splits the flattened native payload back into one
// token slice per publishing service. It performs no validation; the
// classifier owns that.
func parsePMUBootFaultPayload(payload string) pmuBootFaultInfo {
	var info pmuBootFaultInfo
	for _, group := range strings.Split(payload, pmuServiceSeparator) {
		tokens := make([]string, 0, 8)
		for _, token := range strings.Split(group, pmuTokenSeparator) {
			if token == "" {
				continue
			}
			tokens = append(tokens, token)
		}
		info.Groups = append(info.Groups, tokens)
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
