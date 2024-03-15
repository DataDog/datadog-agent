// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package probe holds probe related files
package probe

import (
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"golang.org/x/exp/slices"
)

func newProcScrubber(customSensitiveWords []string) *procutil.DataScrubber {
	scrubber := procutil.NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveWords(customSensitiveWords)

	// token is not always a sensitive word so we cannot change the default sensitive patterns
	// in the case of CWS we can assume token is something we want to scrub so we add it here
	additionals := []string{"*token*"}
	for _, additional := range additionals {
		if !slices.Contains(customSensitiveWords, additional) {
			scrubber.AddCustomSensitiveWords([]string{additional})
		}
	}

	return scrubber
}
