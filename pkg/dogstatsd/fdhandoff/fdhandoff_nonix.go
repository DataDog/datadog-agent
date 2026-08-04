// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !darwin

package fdhandoff

import "os"

// receiveFile returns a "not implemented" error on platforms without SCM_RIGHTS.
func receiveFile(_ string) (*os.File, error) {
	return nil, ErrUnsupported
}
