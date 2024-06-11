// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package replay

import "fmt"

var (
	// DATADOG0F1FF0000 in HEX (D474D060F1FF0000); (F0 | datadogFileVersion) for different file versions support
	// 00 to terminate header
	datadogHeader = []byte{0xD4, 0x74, 0xD0, 0x60, 0xF0, 0xFF, 0x00, 0x00}
	//nolint:revive // TODO(AML) Fix revive linter
	ErrHeaderWrite = fmt.Errorf("capture file header could not be fully written to buffer")
)

const (
	// Version 3+ adds support for nanosecond cadence.
	// Version 2+ adds support for storing state.
	datadogFileVersion uint8 = 3

	versionIndex    = 4
	minStateVersion = 2
	minNanoVersion  = 3
)
