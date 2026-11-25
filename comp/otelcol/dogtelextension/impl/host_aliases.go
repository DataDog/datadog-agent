// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogtelextensionimpl

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/cenkalti/backoff/v7"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
)

// The schedule mirrors the core Agent's host metadata provider
// (comp/metadata/host), and reads the same "metadata_providers" entry: a first
// payload at startup, then intervals growing from earlyInterval to interval.
const (
	hostAliasesProviderName    = "host"
	defaultHostAliasesInterval = 1800 * time.Second
	defaultHostAliasesEarly    = 300 * time.Second
	minHostAliasesInterval     = 60 * time.Second
	maxHostAliasesInterval     = 14400 * time.Second
	hostAliasesJitter          = 0.2
	hostAliasesMultiplier      = 3.0

	// hostAliasesPodsRetry is how long to wait for workloadmeta to know the
	// pods of this node before checking again for a core Agent pod.
	hostAliasesPodsRetry = 30 * time.Second
	// hostAliasesTimeout bounds one collection: the cloud providers' metadata
	// endpoints can take seconds to answer, or to time out.
	hostAliasesTimeout = time.Minute

	// agentComponentLabel is the label the Datadog Helm chart and the Datadog
	// Operator set on node Agent pods, with the value agentComponentNodeAgent.
	agentComponentLabel     = "agent.datadoghq.com/component"
	agentComponentNodeAgent = "agent"
)

// hostAliasesPayload is a host metadata payload ("/intake/") with only the
// agent's hostname and its host aliases, as the core Agent computes them. It
// ties the otel-agent's hostname to the other names telemetry can carry for
// the same host, such as the EC2 instance ID that resource attributes map to.
//
// The backend keeps one alias set per host and replaces it with each payload,
// and it overwrites each metadata field with the latest value it receives.
// This payload carries no other field, so it can't overwrite what another
// sender reports about the host, and every otel-agent on a node sends the same
// one.
type hostAliasesPayload struct {
	InternalHostname string          `json:"internalHostname"`
	Meta             hostAliasesMeta `json:"meta"`
}

type hostAliasesMeta struct {
	Hostname    string   `json:"hostname"`
	HostAliases []string `json:"host_aliases"`
}

// MarshalJSON implements marshaler.JSONMarshaler.
func (p *hostAliasesPayload) MarshalJSON() ([]byte, error) {
	type payload hostAliasesPayload // no MarshalJSON method, so no recursion
	return json.Marshal((*payload)(p))
}

// hostAliasesSender periodically sends a hostAliasesPayload, unless a core
// Agent runs on the same Kubernetes node: that Agent sends the host's aliases
// itself, with its full host metadata, and a second sender would make the
// host's alias set flip between the two payloads.
type hostAliasesSender struct {
	log        log.Component
	hostname   hostnameinterface.Component
	serializer serializer.MetricSerializer

	// Replaced in tests
	isKubernetes   func() bool
	listPods       func() []*workloadmeta.KubernetesPod
	getHostAliases func(context.Context) []string

	backoff     *backoff.ExponentialBackOff
	agentOnNode string // name of the core Agent pod last seen on this node

	cancel context.CancelFunc
	done   chan struct{}
}

func newHostAliasesSender(cfg coreconfig.Component, logger log.Component, hostname hostnameinterface.Component, s serializer.MetricSerializer, wmeta workloadmeta.Component) *hostAliasesSender {
	early, interval := hostAliasesIntervals(cfg, logger)
	bo := &backoff.ExponentialBackOff{
		InitialInterval:     early,
		RandomizationFactor: hostAliasesJitter,
		Multiplier:          hostAliasesMultiplier,
		MaxInterval:         interval,
	}
	bo.Reset()
	listPods := func() []*workloadmeta.KubernetesPod { return nil }
	if wmeta != nil {
		listPods = wmeta.ListKubernetesPods
	}
	return &hostAliasesSender{
		log:          logger,
		hostname:     hostname,
		serializer:   s,
		isKubernetes: func() bool { return env.IsFeaturePresent(env.Kubernetes) },
		listPods:     listPods,
		getHostAliases: func(ctx context.Context) []string {
			aliases, _ := cloudproviders.GetHostAliases(ctx)
			return aliases
		},
		backoff: bo,
	}
}

// hostAliasesIntervals returns the early and the steady-state intervals, from
// the "host" entry of "metadata_providers" when it's valid.
func hostAliasesIntervals(cfg coreconfig.Component, logger log.Component) (early, interval time.Duration) {
	early, interval = defaultHostAliasesEarly, defaultHostAliasesInterval
	providers, err := configutils.GetMetadataProviders(cfg)
	if err != nil {
		logger.Errorf("Error parsing metadata provider configuration, falling back to default host aliases intervals: %s", err)
		return early, interval
	}
	for _, p := range providers {
		if p.Name != hostAliasesProviderName {
			continue
		}
		// Like comp/metadata/host, the configured values are in seconds.
		configured := p.Interval * time.Second
		if configured < minHostAliasesInterval || configured > maxHostAliasesInterval {
			logger.Errorf("Ignoring host metadata interval: %v is outside of accepted values (min: %v, max: %v)", configured, minHostAliasesInterval, maxHostAliasesInterval)
			return early, interval
		}
		interval = configured
		early = min(early, interval)
		if p.EarlyInterval > 0 {
			configuredEarly := p.EarlyInterval * time.Second
			if configuredEarly >= minHostAliasesInterval && configuredEarly <= interval {
				early = configuredEarly
			} else {
				logger.Errorf("Ignoring host metadata early interval: %v must be between %v and the interval %v", configuredEarly, minHostAliasesInterval, interval)
			}
		}
		return early, interval
	}
	return early, interval
}

func (s *hostAliasesSender) start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	go s.run(ctx)
}

func (s *hostAliasesSender) stop() {
	if s.cancel == nil {
		return
	}
	s.cancel()
	<-s.done
}

func (s *hostAliasesSender) run(ctx context.Context) {
	defer close(s.done)
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		timer.Reset(s.collect(ctx))
	}
}

// collect sends the payload if it should, and returns the delay until the
// next collection.
func (s *hostAliasesSender) collect(ctx context.Context) time.Duration {
	if s.isKubernetes() {
		pods := s.listPods()
		// The otel-agent runs in a pod of this node, so until workloadmeta
		// lists at least that one, it can't tell whether a core Agent runs here.
		if len(pods) == 0 {
			s.log.Debug("Not sending host aliases yet: workloadmeta doesn't know the pods of this node")
			return hostAliasesPodsRetry
		}
		if agent := runningNodeAgentPod(pods); agent != "" {
			if agent != s.agentOnNode {
				s.log.Infof("Not sending host aliases: the core Agent pod %s runs on this node and sends them", agent)
			}
			s.agentOnNode = agent
			return s.nextInterval()
		}
		if s.agentOnNode != "" {
			s.log.Infof("The core Agent pod %s no longer runs on this node: sending host aliases", s.agentOnNode)
			s.agentOnNode = ""
		}
	}

	ctx, cancel := context.WithTimeout(ctx, hostAliasesTimeout)
	defer cancel()
	if err := s.send(ctx); err != nil {
		s.log.Warnf("Unable to send host aliases: %s", err)
	}
	return s.nextInterval()
}

func (s *hostAliasesSender) send(ctx context.Context) error {
	hostname, err := s.hostname.Get(ctx)
	if err != nil {
		return err
	}
	var aliases []string
	for _, alias := range s.getHostAliases(ctx) {
		if alias != "" && alias != hostname && !slices.Contains(aliases, alias) {
			aliases = append(aliases, alias)
		}
	}
	if len(aliases) == 0 {
		// Nothing to tie to the hostname. Sending would only make the host
		// exist, under the name the otel-agent's own telemetry already uses.
		s.log.Debugf("Not sending host aliases: none found besides the hostname %q", hostname)
		return nil
	}
	slices.Sort(aliases)
	s.log.Debugf("Sending host aliases %v for hostname %q", aliases, hostname)
	return s.serializer.SendHostMetadata(&hostAliasesPayload{
		InternalHostname: hostname,
		Meta:             hostAliasesMeta{Hostname: hostname, HostAliases: aliases},
	})
}

// nextInterval returns the jittered backoff, capped at the steady-state interval.
func (s *hostAliasesSender) nextInterval() time.Duration {
	next := s.backoff.NextBackOff()
	if next <= 0 || next > s.backoff.MaxInterval {
		return s.backoff.MaxInterval
	}
	return next
}

// runningNodeAgentPod returns the name of a running core Agent pod, as the
// Datadog Helm chart and Operator label it, among the pods of this node.
func runningNodeAgentPod(pods []*workloadmeta.KubernetesPod) string {
	for _, pod := range pods {
		if pod.Labels[agentComponentLabel] == agentComponentNodeAgent && pod.Phase == "Running" {
			return pod.Namespace + "/" + pod.Name
		}
	}
	return ""
}
