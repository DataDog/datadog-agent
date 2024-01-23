// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"time"
)

// Wait attempts to provide a higher precision sleep. It will sleep for larger
// periods, and spin-wait for periods under 1us.
func Wait(d time.Duration) {
	panic("not called")
}
