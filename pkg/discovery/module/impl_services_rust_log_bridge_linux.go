// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package module

import "github.com/DataDog/datadog-agent/pkg/util/log"

// dispatchDiscoveryLog routes a Rust-originated log record to the Go logger.
// The cgo entry point delegates here after rejecting nil/empty/oversize C
// inputs, so this is the unit-testable portion of the bridge. Level filtering
// is applied by pkg/util/log itself, so no pre-gate is needed.
func dispatchDiscoveryLog(level uint32, message string) {
	switch level {
	case 1:
		_ = log.Errorf("[dd_discovery] %s", message)
	case 2:
		_ = log.Warnf("[dd_discovery] %s", message)
	case 3:
		log.Infof("[dd_discovery] %s", message)
	case 4:
		log.Debugf("[dd_discovery] %s", message)
	default:
		log.Tracef("[dd_discovery] %s", message)
	}
}
