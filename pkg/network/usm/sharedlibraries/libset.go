// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package sharedlibraries contains implementation for monitoring of shared libraries opened by other programs
package sharedlibraries

// Libset is a type to represent sets of shared libraries that share the same filtering eBPF program
type Libset string

const (
	// LibsetCrypto is the libset that contains the crypto libraries (libssl, libcrypto, libgnutls)
	LibsetCrypto Libset = "crypto"

	// LibsetGPU contains the libraries for GPU monitoring (libcudart)
	LibsetGPU Libset = "gpu"

	// LibsetLibc is the libset that contains the libc library (libc.so)
	LibsetLibc Libset = "libc"
)

// LibsetToLibSuffixes maps a libset to a list of regexes that match the shared libraries that belong to that libset. Should be
// the same as in the probes.h file
var LibsetToLibSuffixes = map[Libset][]string{
	LibsetCrypto: {"libssl", "crypto", "gnutls"},
	LibsetGPU:    {"libcudart"},
	LibsetLibc:   {"libc"},
}

// IsLibsetValid checks if the given libset is valid (i.e., it's in the LibsetToLibSuffixes map)
func IsLibsetValid(libset Libset) bool {
	_, ok := LibsetToLibSuffixes[libset]
	return ok
}
