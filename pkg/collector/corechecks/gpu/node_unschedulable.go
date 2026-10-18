// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nodeunschedulable"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	agenterrors "github.com/DataDog/datadog-agent/pkg/errors"
	ddmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// This file reports gpu.device.unavailable{unavailable_reason:unschedulable} from the
// node state read from the cluster agent and the gpu.unschedulable_nodes rules.

const (
	nodeUnschedulableEnabledKey         = "gpu.unschedulable_nodes.enabled"
	nodeUnschedulableRefreshIntervalKey = "gpu.unschedulable_nodes.refresh_interval"
)

type nodeUnschedulable struct {
	tracker *nodeunschedulable.Tracker
	series  nodeunschedulable.Series
	source  nodeunschedulable.Source // tests inject one; by default the cluster agent
}

func (c *Check) configureNodeUnschedulable(cfg model.Reader) {
	if !cfg.GetBool(nodeUnschedulableEnabledKey) {
		return
	}
	rules, err := nodeunschedulable.Load(cfg)
	if err != nil {
		// Invalid rules are ignored; a cordoned node is still reported.
		log.Warnf("Ignoring invalid gpu.unschedulable_nodes rules, only cordoned nodes are reported: %v", err)
		rules, _ = nodeunschedulable.Compile(nil, nil)
	}
	c.nodeUnschedulable = &nodeUnschedulable{
		tracker: nodeunschedulable.NewTracker(cfg.GetDuration(nodeUnschedulableRefreshIntervalKey)),
		source:  &dcaUnschedulableSource{check: c, rules: rules},
	}
}

// emitNodeUnschedulable emits the node state on every device of gpu.device.total. Like
// gpu.device.total, it pauses while NVML is released for a MIG reconfiguration.
func (c *Check) emitNodeUnschedulable(snd sender.Sender, now time.Time) {
	nu := c.nodeUnschedulable
	if nu == nil || !env.IsFeaturePresent(env.Kubernetes) {
		return
	}
	nu.tracker.Refresh(nu.source, now)
	outcome := nu.tracker.Outcome()
	points := nu.series.Step(outcome, now)
	if len(points) == 0 {
		return
	}
	devices, err := c.deviceCache.All()
	if err != nil {
		if logLimitCheck.ShouldLog() {
			log.Warnf("error listing devices for the node unschedulable state: %v", err)
		}
		return
	}
	for _, d := range devices {
		uuid := d.GetDeviceInfo().UUID
		if c.isDeviceExcluded(uuid) {
			continue
		}
		deviceTags := c.deviceTags[uuid]
		if len(deviceTags) == 0 {
			deviceTags = []string{"gpu_uuid:" + strings.ToLower(uuid)}
		}
		for _, p := range points {
			tags := append(slices.Clone(deviceTags), p.Tags...)
			m := nvidia.NewMetric("device.unavailable", p.Value, ddmetrics.GaugeType, 0, tags, nil)
			if err := m.Emit(gpuMetricsNs, snd, now); err != nil && logLimitCheck.ShouldLog() {
				log.Warnf("error emitting node-level gpu.device.unavailable: %v", err)
			}
		}
	}
}

// dcaUnschedulableSource reads this node from the cluster agent and matches it
// against the rules.
type dcaUnschedulableSource struct {
	check    *Check
	nodeName string
	rules    *nodeunschedulable.Rules
}

func (s *dcaUnschedulableSource) client() (*clusteragent.DCAClient, string, error) {
	if !pkgconfigsetup.Datadog().GetBool("cluster_agent.enabled") {
		return nil, "", fmt.Errorf("%w: the cluster agent is disabled", nodeunschedulable.ErrUnsupported)
	}
	if s.nodeName == "" {
		if s.check.nodeInfo == nil {
			ni, err := hostinfo.NewNodeInfo()
			if err != nil {
				return nil, "", err
			}
			s.check.nodeInfo = ni
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		name, err := s.check.nodeInfo.GetNodeName(ctx)
		if err != nil {
			return nil, "", err
		}
		s.nodeName = name
	}
	cl, err := clusteragent.GetClusterAgentClient()
	return cl, s.nodeName, err
}

func (s *dcaUnschedulableSource) NodeUnschedulable() (nodeunschedulable.Result, error) {
	cl, node, err := s.client()
	if err != nil {
		return nodeunschedulable.Result{}, err
	}
	st, err := cl.GetNodeStatus(node)
	if err != nil {
		return nodeunschedulable.Result{}, classify(err)
	}
	if st.Conditions == nil {
		// Not an answer of the endpoint: a failed read, the last result is kept.
		return nodeunschedulable.Result{}, errors.New("unexpected answer from the cluster agent node status endpoint")
	}
	var labels map[string]string
	if s.rules.HasLabelRules() {
		if labels, err = cl.GetNodeLabels(node); err != nil {
			return nodeunschedulable.Result{}, err
		}
	}
	conditions := make([]nodeunschedulable.Condition, 0, len(*st.Conditions))
	for _, c := range *st.Conditions {
		conditions = append(conditions, nodeunschedulable.Condition{Type: c.Type, Status: c.Status})
	}
	reasons := s.rules.Evaluate(labels, conditions, st.Unschedulable)
	return nodeunschedulable.Result{Unschedulable: len(reasons) > 0, Reasons: reasons}, nil
}

// Probe requests the node labels, an endpoint that every supported cluster agent
// serves to node agents, with the same client and credentials.
func (s *dcaUnschedulableSource) Probe() error {
	cl, node, err := s.client()
	if err != nil {
		return err
	}
	_, err = cl.GetNodeLabels(node)
	return err
}

// classify marks a 403/404 answer as rejected (how a cluster agent that predates the
// endpoint answers a node agent).
func classify(err error) error {
	if err != nil && agenterrors.IsRemoteService(err) && (strings.Contains(err.Error(), ": 403") || strings.Contains(err.Error(), ": 404")) {
		return fmt.Errorf("%w: %v", nodeunschedulable.ErrRejected, err)
	}
	return err
}
