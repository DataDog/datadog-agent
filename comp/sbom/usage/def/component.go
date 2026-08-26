// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package usage serves the file-to-component table of each SBOM scan to a
// runtime observer, records the usage that observer reports back, and stamps it
// onto the SBOM the agent sends.
package usage

import (
	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"

	"github.com/DataDog/datadog-agent/pkg/sbom/usage"
)

// team: container-integrations

// Component is the component type.
type Component interface {
	// Stamp returns bom carrying the runtime usage reported for the named scan.
	// Components the scan could not tie to a file keep no usage properties at
	// all, so a consumer tells an idle package from an unmeasurable one. It
	// returns bom unchanged when nothing has been reported for the scan yet.
	Stamp(scan usage.ScanID, bom *cyclonedx_v1_4.Bom) *cyclonedx_v1_4.Bom

	// Revision returns how many usage reports have been applied to a scan. A
	// sender that recorded the revision of the payload it last built can tell
	// that the usage has moved on since, which a payload it would otherwise
	// summarise as unchanged has to account for.
	Revision(scan usage.ScanID) uint64

	// Subscribe returns the indexes already published, a channel carrying the
	// ones published next, and a function releasing it.
	Subscribe(size int) ([]*usage.Index, <-chan *usage.Index, func())

	// Report records the usage a consumer observed and returns whether it was
	// applied along with the index currently active for that scan.
	Report(report *usage.Report) usage.ReportAck

	// Capabilities names the scan sources the agent has running, so a consumer
	// learns which indexes to expect rather than inferring it from a
	// configuration key belonging to another process.
	Capabilities() usage.Capabilities

	// Refresh scans the named workload again, because its package database
	// changed and the index no longer describes it. The new index reaches
	// subscribers once the scan completes.
	Refresh(scan usage.ScanID, containerID string)
}
