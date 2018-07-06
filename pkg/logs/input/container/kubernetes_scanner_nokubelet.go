// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !kubelet

package container

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// KubeScanner is not supported on no kubelet environment
type KubeScanner struct{}

// NewKubeScanner returns a new scanner
func NewKubeScanner(sources *config.LogSources, pp pipeline.Provider, auditor *auditor.Auditor) (*KubeScanner, error) {
	return &KubeScanner{}, nil
}

// Start does nothing
func (s *KubeScanner) Start() {}

// Stop does nothing
func (s *KubeScanner) Stop() {}
