// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"fmt"
	"time"
)

// getFormattedTime return readable timestamp
func GetFormattedTime() string {
	now := time.Now()
	local := now.Format("2006-01-02 15:04:05 MST")
	utc := now.UTC().Format("2006-01-02 15:04:05 UTC")
	milliseconds := now.UnixNano() / 1e6
	return fmt.Sprintf("%s / %s (%d)", local, utc, milliseconds)
}
