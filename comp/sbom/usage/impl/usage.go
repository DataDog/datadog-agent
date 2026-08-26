// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package usageimpl implements the SBOM runtime usage component.
package usageimpl

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	status "github.com/DataDog/datadog-agent/comp/core/status"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	usagedef "github.com/DataDog/datadog-agent/comp/sbom/usage/def"
	"github.com/DataDog/datadog-agent/pkg/sbom/usage"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires defines the dependencies of the usage component.
type Requires struct {
	Lifecycle compdef.Lifecycle
	Log       log.Component
	Config    config.Component

	// Source publishes the file-to-component table of every completed scan. It
	// is absent in a build without an SBOM scanner, which leaves the component
	// serving an empty index set and stamping nothing.
	Source option.Option[IndexSource]
}

// Provides defines the output of the usage component.
type Provides struct {
	Comp           usagedef.Component
	FlareProvider  flaretypes.Provider
	StatusProvider status.InformationProvider
}

// IndexSource publishes the file-to-component table of each completed scan. It
// is satisfied by the Trivy collector, which builds the table from the same
// report it marshals into the SBOM.
type IndexSource interface {
	UsageIndexes() <-chan *usage.Index
	Capabilities() usage.Capabilities
	// Rescan reads a live container's filesystem again and publishes the index of
	// what it finds. It reports false when the agent scans no container, in which
	// case there is nothing to read the result of and nothing to stamp it onto.
	Rescan(containerID string) (bool, error)
}

// scan holds everything the agent knows about one workload: the index it last
// published, the usage reported against it, and when each arrived. The table is
// rebuilt on every new index instance, so usage never outlives the BOM it answers.
type scan struct {
	index             *usage.Index
	table             *usage.Table
	indexed           time.Time
	reported          time.Time
	rejectedReports   uint64
	invalidReportRefs map[uint32]struct{}
}

type provider struct {
	log    log.Component
	config config.Component
	source option.Option[IndexSource]

	mu    sync.RWMutex
	scans map[usage.ScanID]*scan

	// subscribers receive every index the agent publishes, so a consumer that
	// connects mid-run gets a snapshot and then the stream.
	subscribers map[chan *usage.Index]struct{}

	// rejectedReports includes reports for unknown scans as well as reports that
	// did not match the active index. Per-index details live on scan.
	rejectedReports uint64
}

// NewComponent creates the usage component.
func NewComponent(reqs Requires) (Provides, error) {
	p := &provider{
		log:         reqs.Log,
		config:      reqs.Config,
		source:      reqs.Source,
		scans:       make(map[usage.ScanID]*scan),
		subscribers: make(map[chan *usage.Index]struct{}),
	}

	if !reqs.Config.GetBool("sbom.enrichment.usage.enabled") {
		return Provides{Comp: p}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			if source, ok := reqs.Source.Get(); ok {
				go p.collect(ctx, source)
			}
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			return nil
		},
	})

	return Provides{
		Comp:           p,
		FlareProvider:  flaretypes.NewProvider(p.fillFlare),
		StatusProvider: status.NewInformationProvider(p),
	}, nil
}

// collect records every index the scanner publishes and hands it to the
// consumers waiting on one.
func (p *provider) collect(ctx context.Context, source IndexSource) {
	indexes := source.UsageIndexes()
	for {
		select {
		case <-ctx.Done():
			return
		case index, ok := <-indexes:
			if !ok {
				return
			}
			p.setIndex(index)
		}
	}
}

// setIndex adopts a new index instance for a scan, dropping the usage held
// against the instance it supersedes, and fans it out to the subscribers.
func (p *provider) setIndex(index *usage.Index) {
	if index == nil {
		return
	}
	p.mu.Lock()
	switch index.Status {
	case usage.Gone:
		delete(p.scans, index.Scan)
	default:
		p.scans[index.Scan] = &scan{
			index:             index,
			table:             usage.NewTable(index),
			indexed:           time.Now(),
			invalidReportRefs: make(map[uint32]struct{}),
		}
	}
	subscribers := make([]chan *usage.Index, 0, len(p.subscribers))
	for ch := range p.subscribers {
		subscribers = append(subscribers, ch)
	}
	p.mu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- index:
		default:
			p.log.Warnf("dropping SBOM index for %s: subscriber is not keeping up", index.Scan)
		}
	}
}

// Subscribe returns the indexes the agent has already published followed by a
// channel carrying the ones it publishes next, and a function releasing it.
func (p *provider) Subscribe(size int) ([]*usage.Index, <-chan *usage.Index, func()) {
	ch := make(chan *usage.Index, size)

	p.mu.Lock()
	snapshot := make([]*usage.Index, 0, len(p.scans))
	for _, s := range p.scans {
		snapshot = append(snapshot, s.index)
	}
	p.subscribers[ch] = struct{}{}
	p.mu.Unlock()

	return snapshot, ch, func() {
		p.mu.Lock()
		delete(p.subscribers, ch)
		p.mu.Unlock()
	}
}

// Report records the usage a consumer observed and identifies the index that
// accepted or rejected it.
func (p *provider) Report(report *usage.Report) usage.ReportAck {
	p.mu.Lock()
	defer p.mu.Unlock()

	ack := usage.ReportAck{}
	if report == nil {
		p.rejectedReports++
		return ack
	}
	ack.Scan = report.Scan
	s, ok := p.scans[report.Scan]
	if !ok {
		p.rejectedReports++
		return ack
	}
	ack.Generation = s.index.Generation
	ack.IndexID = s.index.IndexID
	result := s.table.Apply(report)
	if result.Applied {
		s.reported = time.Now()
		ack.Applied = true
		return ack
	}

	p.rejectedReports++
	s.rejectedReports++
	for _, ref := range result.InvalidRefs {
		s.invalidReportRefs[ref] = struct{}{}
	}
	return ack
}

// Capabilities names the scan sources the agent has running.
func (p *provider) Capabilities() usage.Capabilities {
	if source, ok := p.source.Get(); ok {
		return source.Capabilities()
	}
	return usage.Capabilities{}
}

// Refresh implements usagedef.Component.
//
// A container that wrote its package database no longer matches the index built
// for it: a package may have been added, or replaced under a file already
// observed. Where the agent scans containers, the container's filesystem is read
// again and the fresh table supersedes the one it diverged from, which is the
// only way the components themselves catch up.
//
// Where it scans only images there is nothing to read: an image is immutable, so
// rescanning it would return the table it already has, and no payload exists for
// a container-scoped one to be stamped onto. The index is then re-published under
// a new generation, which at least drops usage that can no longer be trusted.
func (p *provider) Refresh(scanID usage.ScanID, containerID string) {
	if source, ok := p.source.Get(); ok && containerID != "" {
		rescanning, err := source.Rescan(containerID)
		if err != nil {
			p.log.Warnf("could not rescan container %s: %v", containerID, err)
		}
		if rescanning {
			p.log.Debugf("rescanning container %s: it wrote its package database", containerID)
			return
		}
	}

	p.mu.Lock()
	s, ok := p.scans[scanID]
	if !ok {
		p.mu.Unlock()
		return
	}
	next := *s.index
	next.Generation = s.index.Generation + 1
	p.mu.Unlock()

	p.log.Debugf("invalidating SBOM usage for %s: container %s wrote its package database", scanID, containerID)
	p.setIndex(&next)
}

// Revision implements usagedef.Component.
func (p *provider) Revision(scanID usage.ScanID) uint64 {
	p.mu.RLock()
	s, ok := p.scans[scanID]
	p.mu.RUnlock()
	if !ok {
		return 0
	}
	return s.table.Revision()
}

// Stamp implements usagedef.Component.
func (p *provider) Stamp(scanID usage.ScanID, bom *cyclonedx_v1_4.Bom) *cyclonedx_v1_4.Bom {
	p.mu.RLock()
	s, ok := p.scans[scanID]
	p.mu.RUnlock()
	if !ok {
		return bom
	}
	return s.table.Stamp(bom)
}
