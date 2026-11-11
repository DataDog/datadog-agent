// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package configimpl

import (
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

// setupKubeClusterUID is a no-op: serverless agents do not resolve the cluster UID.
func setupKubeClusterUID(*config.AgentConfig, compdef.Lifecycle) {}
