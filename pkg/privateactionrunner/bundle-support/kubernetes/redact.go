// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package kubernetes

import (
	"github.com/DataDog/datadog-agent/pkg/redact"
)

// ScrubItems scrubs each list item in place.
func ScrubItems[T any](items []T, scrub func(*T, *redact.DataScrubber)) {
	scrubber := redact.NewDefaultDataScrubber()
	for i := range items {
		scrub(&items[i], scrubber)
	}
}
