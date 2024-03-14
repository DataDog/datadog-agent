// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubeapiserver

// Package clusteragent fetch information about the cluster agent
package clusteragent

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetDCAStatus empty function for agents not running in a  k8s environment
func GetDCAStatus(_ map[string]interface{}) {
	log.Info("Not implemented")
}

// GetProvider returns nil
func GetProvider(_ config.Component) status.Provider {
	return nil
}
