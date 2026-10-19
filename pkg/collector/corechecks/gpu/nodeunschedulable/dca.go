// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodeunschedulable

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	agenterrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// EnabledConfigKey turns the reporting of unschedulable nodes on.
	EnabledConfigKey = "gpu.unschedulable_nodes.enabled"
	// RefreshIntervalConfigKey is how often the node state is read from the cluster agent.
	RefreshIntervalConfigKey = "gpu.unschedulable_nodes.refresh_interval"
)

// New returns a cache that reads this node from the cluster agent, or nil when
// gpu.unschedulable_nodes is disabled. It is shared by every GPU vendor check.
func New(cfg model.Reader) *Cache {
	if !cfg.GetBool(EnabledConfigKey) {
		return nil
	}
	src := &dcaSource{cfg: cfg, rules: loadRules(cfg)}
	return NewCache(src, cfg.GetDuration(RefreshIntervalConfigKey))
}

// loadRules ignores invalid rules; a cordoned node is still reported.
func loadRules(cfg model.Reader) *Rules {
	rules, err := Load(cfg)
	if err != nil {
		log.Warnf("Ignoring invalid gpu.unschedulable_nodes rules, only cordoned nodes are reported: %v", err)
		rules, _ = Compile(nil, nil)
	}
	return rules
}

// dcaSource reads this node from the cluster agent and matches it against the rules.
type dcaSource struct {
	cfg      model.Reader
	rules    *Rules
	nodeInfo *hostinfo.NodeInfo
	nodeName string
}

func (s *dcaSource) client() (*clusteragent.DCAClient, string, error) {
	if !s.cfg.GetBool("cluster_agent.enabled") {
		return nil, "", fmt.Errorf("%w: the cluster agent is disabled", ErrUnsupported)
	}
	if s.nodeName == "" {
		if s.nodeInfo == nil {
			ni, err := hostinfo.NewNodeInfo()
			if err != nil {
				return nil, "", err
			}
			s.nodeInfo = ni
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		name, err := s.nodeInfo.GetNodeName(ctx)
		if err != nil {
			return nil, "", err
		}
		s.nodeName = name
	}
	cl, err := clusteragent.GetClusterAgentClient()
	return cl, s.nodeName, err
}

func (s *dcaSource) NodeUnschedulable() (Result, error) {
	cl, node, err := s.client()
	if err != nil {
		return Result{}, err
	}
	st, err := cl.GetNodeStatus(node)
	if err != nil {
		return Result{}, classify(err)
	}
	if st.Conditions == nil {
		// Not an answer of the endpoint: a failed read, the last result is kept.
		return Result{}, errors.New("unexpected answer from the cluster agent node status endpoint")
	}
	var labels map[string]string
	if s.rules.HasLabelRules() {
		if labels, err = cl.GetNodeLabels(node); err != nil {
			return Result{}, err
		}
	}
	conditions := make([]Condition, 0, len(*st.Conditions))
	for _, c := range *st.Conditions {
		conditions = append(conditions, Condition{Type: c.Type, Status: c.Status})
	}
	reasons := s.rules.Evaluate(labels, conditions, st.Unschedulable)
	return Result{Unschedulable: len(reasons) > 0, Reasons: reasons}, nil
}

// Probe requests the node labels, an endpoint that every supported cluster agent
// serves to node agents, with the same client and credentials.
func (s *dcaSource) Probe() error {
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
		return fmt.Errorf("%w: %v", ErrRejected, err)
	}
	return err
}
