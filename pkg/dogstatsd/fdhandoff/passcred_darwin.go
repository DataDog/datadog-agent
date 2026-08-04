// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fdhandoff

import "syscall"

// setPassCred is a no-op on darwin: there is no SO_PASSCRED equivalent, and
// DogStatsD origin detection is a Linux-only feature anyway.
func setPassCred(_ syscall.RawConn) error {
	return nil
}
