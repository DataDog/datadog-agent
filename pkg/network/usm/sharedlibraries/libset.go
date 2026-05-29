// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package sharedlibraries contains implementation for monitoring of shared libraries opened by other programs
package sharedlibraries

import (
	sharedlibtypes "github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/types"
)

// Libset is a type alias for backward compatibility.
// New code should import pkg/network/usm/sharedlibraries/types directly.
type Libset = sharedlibtypes.Libset

const (
	// LibsetCrypto is the libset that contains the crypto libraries
	LibsetCrypto = sharedlibtypes.LibsetCrypto
	// LibsetGPU contains the libraries for GPU monitoring
	LibsetGPU = sharedlibtypes.LibsetGPU
	// LibsetLibc is the libset that contains the libc library
	LibsetLibc = sharedlibtypes.LibsetLibc
)

// LibsetToLibSuffixes maps a libset to a list of regexes that match the shared libraries
var LibsetToLibSuffixes = sharedlibtypes.LibsetToLibSuffixes

// IsLibsetValid checks if the given libset is valid
func IsLibsetValid(libset Libset) bool {
	return sharedlibtypes.IsLibsetValid(libset)
}
