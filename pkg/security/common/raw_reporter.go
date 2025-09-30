// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import "time"

// RawReporter defines an interface for reporting raw rule events
type RawReporter interface {
	ReportRaw(content []byte, service string, timestamp time.Time, tags ...string)
}
