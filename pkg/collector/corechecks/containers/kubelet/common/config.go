// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package common

import (
	"gopkg.in/yaml.v2"
)

const (
	KubeletMetricsPrefix = "kubernetes_core."
)

// KubeletConfig is the config of the Kubelet.
type KubeletConfig struct {
	Tags []string `yaml:"tags"`
}

func (c *KubeletConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}
