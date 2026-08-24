// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package opener

import "os"

func isOpenFlagsUnsupportedError(error) bool {
	return false
}

func openDirect(string) (*os.File, error) {
	return nil, ErrOpenFlagsUnsupported
}

func directIOAlignments(*os.File) (int, int) {
	return directIOAlignment, directIOAlignment
}
