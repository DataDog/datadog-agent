// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package usageimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/sbom/usage"
)

// componentUsage is one component a runtime observer reported running.
type componentUsage struct {
	BOMRef   string `json:"bom_ref"`
	Purl     string `json:"purl"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	LastSeen string `json:"last_seen"`
	Suid     bool   `json:"setuid"`
	AsRoot   bool   `json:"as_root"`
}

// scanUsage is the flare view of one workload: what the agent published, what
// came back, and whether the two agree.
//
// It records the inputs of the stamp rather than its output, so the SBOM in the
// flare stays the single copy the agent holds and the payload it sent stays
// reproducible from the two. It also separates what the observer said from what
// the agent did with it, which a stamped BOM alone cannot show.
type scanUsage struct {
	Scan               string           `json:"scan"`
	Generation         uint64           `json:"generation"`
	IndexID            string           `json:"index_id"`
	Source             string           `json:"source"`
	Indexed            string           `json:"indexed"`
	Reported           string           `json:"reported,omitempty"`
	Components         int              `json:"components"`
	Anchored           int              `json:"anchored"`
	Files              int              `json:"files"`
	UnmappedComponents int              `json:"unmapped_components"`
	HashCollisions     int              `json:"hash_collisions"`
	RejectedReports    uint64           `json:"rejected_reports"`
	InvalidReportRefs  []uint32         `json:"invalid_report_refs"`
	Usage              []componentUsage `json:"usage"`
}

var unsafeFileChars = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

// fillFlare writes one file per scanned workload.
func (p *provider) fillFlare(_ context.Context, fb flaretypes.FlareBuilder) error {
	for _, view := range p.views() {
		content, err := json.MarshalIndent(view, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal usage for %s: %w", view.Scan, err)
		}
		name := unsafeFileChars.ReplaceAllString(view.Scan, "_")
		_ = fb.AddFileWithoutScrubbing(filepath.Join("sbom-usage", name+".json"), content)
	}
	return nil
}

// views renders every scan the agent holds, newest index first.
func (p *provider) views() []scanUsage {
	p.mu.RLock()
	defer p.mu.RUnlock()

	source := "none"
	if caps := p.Capabilities(); caps.Any() {
		source = fmt.Sprintf("container_image=%t container=%t host=%t",
			caps.ContainerImage, caps.Container, caps.Host)
	}

	views := make([]scanUsage, 0, len(p.scans))
	for id, s := range p.scans {
		view := scanUsage{
			Scan:               string(id),
			Generation:         s.index.Generation,
			IndexID:            s.index.IndexID,
			Source:             source,
			Indexed:            s.indexed.UTC().Format(time.RFC3339),
			Components:         len(s.index.Components),
			Anchored:           s.table.Anchored(),
			Files:              len(s.index.Refs),
			UnmappedComponents: s.index.UnmappedComponents,
			HashCollisions:     s.index.HashCollisions,
			RejectedReports:    s.rejectedReports,
			InvalidReportRefs:  make([]uint32, 0, len(s.invalidReportRefs)),
			Usage:              []componentUsage{},
		}
		for ref := range s.invalidReportRefs {
			view.InvalidReportRefs = append(view.InvalidReportRefs, ref)
		}
		if !s.reported.IsZero() {
			view.Reported = s.reported.UTC().Format(time.RFC3339)
		}

		byBOMRef := make(map[string]*usage.Component, len(s.index.Components))
		for i := range s.index.Components {
			if bomRef := s.index.Components[i].BOMRef; bomRef != "" {
				byBOMRef[bomRef] = &s.index.Components[i]
			}
		}

		for bomRef, u := range s.table.Seen() {
			comp, ok := byBOMRef[bomRef]
			if !ok {
				continue
			}
			view.Usage = append(view.Usage, componentUsage{
				BOMRef:   bomRef,
				Purl:     comp.Purl,
				Name:     comp.Name,
				Version:  comp.Version,
				LastSeen: u.LastSeen.UTC().Format(time.RFC3339),
				Suid:     u.Suid,
				AsRoot:   u.AsRoot,
			})
		}

		slices.SortFunc(view.Usage, func(a, b componentUsage) int {
			if order := cmpString(a.BOMRef, b.BOMRef); order != 0 {
				return order
			}
			return cmpString(a.Purl, b.Purl)
		})
		slices.Sort(view.InvalidReportRefs)
		views = append(views, view)
	}

	slices.SortFunc(views, func(a, b scanUsage) int { return cmpString(a.Scan, b.Scan) })
	return views
}

func cmpString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Name implements status.Provider.
func (p *provider) Name() string { return "SBOM usage" }

// Section implements status.Provider.
func (p *provider) Section() string { return "SBOM" }

// JSON implements status.Provider.
func (p *provider) JSON(_ bool, stats map[string]interface{}) error {
	stats["sbomUsage"] = p.stats()
	return nil
}

// Text implements status.Provider.
func (p *provider) Text(_ bool, buffer io.Writer) error {
	s := p.stats()
	_, err := fmt.Fprintf(buffer, "  Runtime usage enrichment: %s\n    Sources: %s\n    Workloads indexed: %d\n    Components in use: %d\n    Rejected reports: %d\n    Last report: %s\n",
		s["state"], s["sources"], s["workloads"], s["inUse"], s["rejectedReports"], s["lastReport"])
	return err
}

// HTML implements status.Provider.
func (p *provider) HTML(_ bool, _ io.Writer) error { return nil }

// stats summarises the enrichment, so the first place anyone looks says whether
// it is running rather than leaving them to infer it from an unstamped SBOM.
func (p *provider) stats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	state := "disabled"
	if p.config.GetBool("sbom.enrichment.usage.enabled") {
		state = "enabled"
	}

	caps := p.Capabilities()
	sources := "none"
	if caps.Any() {
		sources = fmt.Sprintf("container_image=%t container=%t host=%t",
			caps.ContainerImage, caps.Container, caps.Host)
	}

	inUse := 0
	lastReport := time.Time{}
	for _, s := range p.scans {
		inUse += len(s.table.Seen())
		if s.reported.After(lastReport) {
			lastReport = s.reported
		}
	}

	last := "never"
	if !lastReport.IsZero() {
		last = time.Since(lastReport).Truncate(time.Second).String() + " ago"
	}

	return map[string]interface{}{
		"state":           state,
		"sources":         sources,
		"workloads":       len(p.scans),
		"inUse":           inUse,
		"rejectedReports": p.rejectedReports,
		"lastReport":      last,
	}
}
