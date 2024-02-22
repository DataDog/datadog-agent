// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package config

import (
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

func containerTagsFunc(cid string) ([]string, error) {
	return tagger.Tag("container_id://"+cid, collectors.HighCardinality)
}

func containerSetup(cfg *config.AgentConfig, procRoot string) {
	cfg.ContainerTags = containerTagsFunc
	cfg.ContainerProcRoot = procRoot
}
