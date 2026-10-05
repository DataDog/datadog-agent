// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package run

import (
	"context"
	"fmt"
	"sync"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	pkgconfigenv "github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ec2FallbackProvider is the provider name reported when the hostname comes from the
// EC2 instance ID fallback. It matches the name used by pkg/util/hostname for EC2.
const ec2FallbackProvider = "aws"

// standaloneHostname resolves the hostname locally in standalone mode, where there is
// no core agent to ask.
//
// It runs the regular provider chain first. When that finds nothing and the otel-agent
// runs in a Kubernetes pod without a configured kubelet host (the default for the
// OpenTelemetry Operator and Collector Helm chart deployments), the kubelet provider
// can't run, the OS hostname is the pod name and the EC2 provider is skipped because no
// default EC2 hostname was found. In that case it falls back to the EC2 instance ID,
// which is also what the core agent reports on EKS, where kubelet node names are
// default EC2 hostnames.
//
// The fallback is not used when kubernetes_kubelet_host is set: a kubelet failure then
// keeps failing startup, so that a transient kubelet outage can't switch the hostname
// of a node with a custom name between restarts.
type standaloneHostname struct {
	hostnameinterface.Component

	cfg           coreconfig.Component
	getInstanceID func(context.Context) (string, error)

	mu       sync.Mutex
	fallback *hostnameinterface.Data
}

func newStandaloneHostname(cfg coreconfig.Component) hostnameinterface.Component {
	return &standaloneHostname{
		Component:     hostnameimpl.NewHostnameService(),
		cfg:           cfg,
		getInstanceID: ec2.GetInstanceID,
	}
}

// Get returns the hostname.
func (s *standaloneHostname) Get(ctx context.Context) (string, error) {
	data, err := s.GetWithProvider(ctx)
	return data.Hostname, err
}

// GetSafe returns the hostname, or 'unknown host' if anything goes wrong.
func (s *standaloneHostname) GetSafe(ctx context.Context) string {
	name, err := s.Get(ctx)
	if err != nil {
		return "unknown host"
	}
	return name
}

// GetWithProvider returns the hostname and the provider that was used to retrieve it.
func (s *standaloneHostname) GetWithProvider(ctx context.Context) (hostnameinterface.Data, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Once the fallback was used, keep returning it so the hostname stays stable.
	if s.fallback != nil {
		return *s.fallback, nil
	}

	data, err := s.Component.GetWithProvider(ctx)
	if err == nil {
		return data, nil
	}

	if !pkgconfigenv.IsKubernetes() || s.cfg.GetString("kubernetes_kubelet_host") != "" {
		return data, err
	}

	instanceID, ec2Err := s.getInstanceID(ctx)
	if ec2Err != nil {
		log.Debugf("Unable to use the EC2 instance ID as hostname fallback: %s", ec2Err)
		return data, err
	}
	if validErr := validate.ValidHostname(instanceID); validErr != nil {
		log.Debugf("EC2 instance ID '%s' is not a valid hostname: %s", instanceID, validErr)
		return data, err
	}

	log.Infof("Hostname providers found no hostname (%s) and kubernetes_kubelet_host is not set; using the EC2 instance ID '%s' as hostname", err, instanceID)
	s.fallback = &hostnameinterface.Data{Hostname: instanceID, Provider: ec2FallbackProvider}
	return *s.fallback, nil
}

// setStandaloneTraceHostname makes the trace config use the hostname resolved by the
// hostname component. It must only be used in standalone mode.
//
// Without it, the trace config resolves the hostname on its own by asking the core
// agent over IPC, then by running the core agent binary, and finally falls back to the
// OS hostname, which it refuses inside a container. In standalone mode there is no core
// agent, so startup fails in Kubernetes unless hostname is set explicitly.
func setStandaloneTraceHostname(ctx context.Context, cfg coreconfig.Component, h hostnameinterface.Component) error {
	if cfg.IsConfigured("hostname") {
		return nil
	}
	hostname, err := h.Get(ctx)
	if err != nil {
		return fmt.Errorf("unable to get hostname for the trace agent: %w", err)
	}
	cfg.Set("hostname", hostname, pkgconfigmodel.SourceAgentRuntime)
	return nil
}
