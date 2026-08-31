// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package file

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// fingerprintOpenFlagsInfoKey groups the per-file open_flags failures of a
// source under a single heading on the status page.
const fingerprintOpenFlagsInfoKey = "Fingerprint Open Flags Failures"

// openFlagsErrorReporter surfaces files that cannot be fingerprinted with the
// open_flags their source configured. Such a file without a tailer stays
// unscheduled; one with an active tailer keeps reading its open descriptor but
// cannot detect rotation until fingerprinting recovers.
//
// Failures are recorded on the source rather than on a tailer because a file
// that was never picked up has no tailer, which is the whole point: without this
// the file just disappears from the status page with no explanation.
//
// The reported set is scoped to a scan: reset drops the previous result and the
// scan reports whatever is still failing, so a file that recovered or
// disappeared leaves nothing behind.
type openFlagsErrorReporter struct {
	// reported holds the sources written to since the last reset, so a scan retracts exactly
	// what the previous one recorded; the scanned source list can omit an active tailer's source.
	reported []*sources.ReplaceableSource

	// logLimit rate limits the warning for an unusable open_flags configuration.
	// A persistent failure is retried on every scan, so without this the warning
	// would repeat for the lifetime of the Agent.
	logLimit *log.Limit
}

func newOpenFlagsErrorReporter() *openFlagsErrorReporter {
	return &openFlagsErrorReporter{logLimit: log.NewLogLimit(5, 10*time.Minute)}
}

// reset drops the failures recorded since the previous scan.
func (r *openFlagsErrorReporter) reset() {
	for _, source := range r.reported {
		if info, ok := source.GetInfo(fingerprintOpenFlagsInfoKey).(*status.MappedInfo); ok {
			info.Clear()
		}
	}
	clear(r.reported)
	r.reported = r.reported[:0]
}

// report records that file cannot be fingerprinted with the open_flags its
// source configured.
func (r *openFlagsErrorReporter) report(file *tailer.File, err error) {
	info, ok := file.Source.GetInfo(fingerprintOpenFlagsInfoKey).(*status.MappedInfo)
	if !ok {
		info = status.NewMappedInfo(fingerprintOpenFlagsInfoKey)
		file.Source.RegisterInfo(info)
	}
	info.SetMessage(file.Path, fmt.Sprintf("Fingerprinting with the configured open flags failed for this file: %v", err))
	r.reported = append(r.reported, file.Source)

	if r.logLimit.ShouldLog() {
		log.Warnf(
			"Fingerprinting with the configured open_flags failed for %q (%v). "+
				"An active tailer will keep reading its open descriptor, but rotation detection is unavailable until fingerprinting recovers. "+
				"Remove open_flags from the source, or scope the source so it only matches files that support them.",
			file.Path,
			err,
		)
	}
}
