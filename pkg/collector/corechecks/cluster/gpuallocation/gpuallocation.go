// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// Package gpuallocation implements the GPU allocation cluster check, which
// reports how long workloads wait for accelerators and how many accelerators
// are handed out.
//
// The check runs in the Cluster Agent because it reads cluster-wide scheduling
// state: the questions it answers ("how long did this job queue for GPUs",
// "how many devices are in use on each node") cannot be answered from a single
// node, and for a workload that is still waiting there is no container on any
// node to observe yet.
package gpuallocation

import (
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster"
	"github.com/DataDog/datadog-agent/pkg/config/helper"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "gpu_allocation"

	// metricsNs is a sub-namespace of the node-level `gpu.` metrics. The
	// node-level check reports device utilisation; these report scheduling and
	// allocation, which is why they are namespaced apart.
	metricsNs = "gpu.allocation."
)

// Check reports GPU allocation state from the Cluster Agent's workloadmeta.
type Check struct {
	core.CheckBase
	store             workloadmeta.Component
	backends          []backend
	runLeaderElection bool
	// now and isLeader are overridable in tests.
	now      func() time.Time
	isLeader func() (bool, error)

	// previousPendingNamespaces and previousAllocatedNodes remember which tag
	// values (per backend) were reported last Run. A Gauge that stops receiving
	// samples for a tag combination does not read as zero to a monitor -- it
	// either sticks on its last value or goes to no-data, neither of which
	// clears an alert -- so a namespace/node that drops out of this Run's
	// results must still get one explicit zero before being forgotten.
	previousPendingNamespaces map[string]map[string]struct{}
	previousAllocatedNodes    map[string]map[string]struct{}
}

// Factory creates a new check factory
func Factory(store workloadmeta.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check { return newCheck(store) })
}

func newCheck(store workloadmeta.Component) check.Check {
	acceleratorClasses := make(map[string]struct{})
	for _, class := range pkgconfigsetup.Datadog().GetStringSlice("cluster_agent.gpu_allocation.device_classes") {
		acceleratorClasses[class] = struct{}{}
	}

	return &Check{
		CheckBase: core.NewCheckBase(CheckName),
		store:     store,
		backends: []backend{
			&draBackend{store: store, acceleratorClasses: acceleratorClasses},
		},
		runLeaderElection:         !helper.IsCLCRunner(pkgconfigsetup.Datadog()),
		now:                       time.Now,
		isLeader:                  isLeader,
		previousPendingNamespaces: make(map[string]map[string]struct{}),
		previousAllocatedNodes:    make(map[string]map[string]struct{}),
	}
}

// Configure configures the GPU allocation check
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, config, initConfig integration.Data, source string, provider string) error {
	// Opt-in: only run where DRA accelerator allocation is actually wanted.
	//
	// This must wrap ErrSkipCheckInstance rather than return a plain error. The
	// shipped conf.yaml.default declares an instance unconditionally, so every
	// default Cluster Agent configures this check; the loader logs an ordinary
	// error at ERROR level, which would mean an error line on every cluster that
	// has not opted in. ErrSkipCheckInstance is the loader's contract for "this
	// instance does not apply here" and is skipped silently.
	if !pkgconfigsetup.Datadog().GetBool("cluster_agent.gpu_allocation.enabled") {
		return fmt.Errorf("%w: GPU allocation check is disabled", check.ErrSkipCheckInstance)
	}

	c.BuildID(integrationConfigDigest, config, initConfig)
	return c.CommonConfigure(senderManager, initConfig, config, source, provider)
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	defer sender.Commit()

	// Every replica holds the same cluster-wide view, so without this guard a
	// multi-replica Cluster Agent would report each allocation more than once.
	//
	// A follower still walks the state below (so it can take over cleanly) but
	// publishes nothing, and -- critically -- does not advance the remembered
	// tag sets. Those sets record what was *last published*; if a follower
	// replaced them with the current view it would forget the series that
	// disappeared while it was not publishing, and a replica promoted after a
	// workload vanished would never zero it.
	publish := true
	if c.runLeaderElection {
		isCurrentLeader, err := c.isLeader()
		if err != nil {
			return err
		}
		if !isCurrentLeader {
			log.Debugf("Not leader. Observing state without publishing for the GPU allocation check")
			publish = false
		}
	}

	if c.previousPendingNamespaces == nil {
		c.previousPendingNamespaces = make(map[string]map[string]struct{})
	}
	if c.previousAllocatedNodes == nil {
		c.previousAllocatedNodes = make(map[string]map[string]struct{})
	}

	now := c.now()
	for _, b := range c.backends {
		source := b.name()
		sourceTag := "source:" + source

		// Aggregate per namespace rather than emitting one series per claim or
		// pod: workload names are high cardinality and churn on every restart.
		pendingCount := make(map[string]int)
		longestWait := make(map[string]float64)
		for _, p := range b.pending(now) {
			pendingCount[p.namespace]++
			if waited := p.waiting.Seconds(); waited > longestWait[p.namespace] {
				longestWait[p.namespace] = waited
			}
		}

		currentNamespaces := make(map[string]struct{}, len(pendingCount))
		for namespace, count := range pendingCount {
			currentNamespaces[namespace] = struct{}{}
			if !publish {
				continue
			}
			tags := []string{sourceTag, "kube_namespace:" + namespace}
			sender.Gauge(metricsNs+"pending.count", float64(count), "", tags)
			// The longest current wait is the alertable number: it answers
			// "is something stuck waiting for GPUs right now".
			sender.Gauge(metricsNs+"pending.seconds.max", longestWait[namespace], "", tags)
		}
		// A namespace reported last Run but absent now is no longer waiting.
		// Report it as zero once, so a monitor alerting on it can recover,
		// then stop tracking it -- otherwise a namespace gone for good would
		// accumulate as permanent, ever-repeated cardinality.
		for namespace := range c.previousPendingNamespaces[source] {
			if _, stillPending := currentNamespaces[namespace]; stillPending {
				continue
			}
			if !publish {
				continue
			}
			tags := []string{sourceTag, "kube_namespace:" + namespace}
			sender.Gauge(metricsNs+"pending.count", 0, "", tags)
			sender.Gauge(metricsNs+"pending.seconds.max", 0, "", tags)
		}
		if publish {
			c.previousPendingNamespaces[source] = currentNamespaces
		}

		currentNodes := make(map[string]struct{})
		for _, a := range b.allocated() {
			currentNodes[a.node] = struct{}{}
			if !publish {
				continue
			}
			sender.Gauge(metricsNs+"devices.allocated", float64(a.count), "",
				[]string{sourceTag, "kube_node:" + a.node})
		}
		// Same reasoning as above: a node that no longer has any allocated
		// accelerators must read as zero, not as silence.
		for node := range c.previousAllocatedNodes[source] {
			if _, stillAllocated := currentNodes[node]; stillAllocated {
				continue
			}
			if !publish {
				continue
			}
			sender.Gauge(metricsNs+"devices.allocated", 0, "", []string{sourceTag, "kube_node:" + node})
		}
		if publish {
			c.previousAllocatedNodes[source] = currentNodes
		}
	}

	return nil
}

// isLeader mirrors the other Cluster Agent core checks: without leader election
// a replica cannot tell whether it is the only one reporting, so the check
// refuses to run rather than risk double-counting.
func isLeader() (bool, error) {
	if !pkgconfigsetup.Datadog().GetBool("leader_election") {
		return false, errors.New("leader election not enabled. The check will not run")
	}

	if _, err := cluster.RunLeaderElection(); err != nil {
		if err == apiserver.ErrNotLeader {
			return false, nil
		}
		return false, err
	}

	return true, nil
}
