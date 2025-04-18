// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && !trivy

// Package sbom holds sbom related files
package sbom

import (
	"context"

	"github.com/DataDog/datadog-go/v5/statsd"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// Event defines the SBOM event type
type Event int

const (
	// SBOMComputed is used to notify that a SBOM was computed
	SBOMComputed Event = iota + 1
)

// Resolver is the Software Bill-Of-material resolver
type Resolver struct {
	*utils.Notifier[Event, *sbom.ScanResult]
}

// NewResolver returns a new instance of a SBOM resolver
func NewResolver(_ *config.RuntimeSecurityConfig, _ statsd.ClientInterface, _ workloadmeta.Component) (*Resolver, error) {
	return &Resolver{}, nil
}

// OnCGroupDeletedEvent is used to handle a CGroupDeleted event
func (r *Resolver) OnCGroupDeletedEvent(_ *cgroupModel.CacheEntry) {
}

// OnWorkloadSelectorResolvedEvent is used to handle the creation of a new cgroup with its resolved tags
func (r *Resolver) OnWorkloadSelectorResolvedEvent(_ *tags.Workload) {
}

// ResolvePackage returns the Package that owns the provided file
func (r *Resolver) ResolvePackage(_ containerutils.ContainerID, _ *model.FileEvent) *Package {
	return nil
}

// SendStats sends stats
func (r *Resolver) SendStats() error {
	return nil
}

// Start starts the goroutine of the SBOM resolver
func (r *Resolver) Start(_ context.Context) error {
	return nil
}

// RefreshSBOM regenerates a SBOM for a container
func (r *Resolver) RefreshSBOM(_ containerutils.ContainerID) error {
	return nil
}
