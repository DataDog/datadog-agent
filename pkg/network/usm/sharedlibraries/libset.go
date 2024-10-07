// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package sharedlibraries

// Libset represents the name of a set of shared libraries that share the same filtering eBPF program
type Libset string

const (
	// LibsetCrypto is the libset that contains the crypto libraries (libssl, libcrypto, libgnutls)
	LibsetCrypto Libset = "crypto"
)

// LibsetToLibSuffixes maps a libset to a list of regexes that match the shared libraries that belong to that libset. Should be
// the same as in the probes.h file
var LibsetToLibSuffixes = map[Libset][]string{
	LibsetCrypto: {"libssl", "crypto", "gnutls"},
}
