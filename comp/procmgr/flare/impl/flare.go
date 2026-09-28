// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package flareimpl adds dd-procmgrd supervision state to flares.
//
// Agent processes that moved off the SCM onto dd-procmgrd leave their legacy service behind,
// registered and permanently Stopped. Without this provider a flare shows those stopped services
// and nothing about the supervisor that actually owns the processes.
package flareimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/procmgr/coat"
)

// flareFile is where the supervision dump lands in the archive. dd-procmgr.log is not written
// here: it already reaches logs/ through the flare's log directory sweep.
const flareFile = "procmgr/state.json"

// Provides specifies the types returned by the constructor.
type Provides struct {
	FlareProvider flaretypes.Provider
}

// reporter is the subset of coat.Collector this component needs, so tests can substitute a fake
// rather than requiring a live dd-procmgrd.
type reporter interface {
	Report(ctx context.Context) coat.SupportReport
}

type procmgrFlare struct {
	reporter reporter
}

// NewComponent returns a flare provider that dumps dd-procmgrd supervision state.
func NewComponent() Provides {
	return newProvides(coat.NewCollector())
}

func newProvides(r reporter) Provides {
	p := &procmgrFlare{reporter: r}
	return Provides{FlareProvider: flaretypes.NewProvider(p.fillFlare)}
}

// fillFlare writes the supervision dump. It reports no error for an unreachable daemon: coat
// records that as data in the report, because "dd-procmgrd is not answering" is what a support
// engineer needs to read, and a missing file would instead look like procmgr was never asked.
func (p *procmgrFlare) fillFlare(ctx context.Context, fb flaretypes.FlareBuilder) error {
	report := p.reporter.Report(ctx)

	// AddFile scrubs too, but line by line, which cannot pair a "--password" argument with its
	// value on the next line of a JSON array. Scrub here, where the argv is still a slice.
	report.Scrub()

	content, err := marshalReport(report)
	if err != nil {
		return fb.AddFile(flareFile, []byte(fmt.Sprintf("could not serialize dd-procmgrd state: %v", err)))
	}

	return fb.AddFile(flareFile, content)
}

// marshalReport renders the report for a human reader. HTML escaping is off because the notes
// compare states with ">" and json.Marshal would turn that into \u003e.
func marshalReport(report coat.SupportReport) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
