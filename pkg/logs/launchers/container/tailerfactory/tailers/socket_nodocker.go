// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !docker

package tailers

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func NewDockerSocketTailer(dockerutil *dockerutilPkg.DockerUtil, containerID string, source *sources.LogSource, pipeline chan *message.Message, readTimeout time.Duration, registry auditor.Registry, tagger tagger.Component) *DockerSocketTailer {
	return &DockerSocketTailer{}
}

func (t *DockerSocketTailer) Start() error {}
