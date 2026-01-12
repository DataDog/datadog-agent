// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && !docker

// Package tailerfactory implements the logic required to determine which kind
// of tailer to use for a container-related LogSource, and to create that tailer.
package tailerfactory

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers/container"
)

func (tf *factory) makeSocketTailer(_ *sources.LogSource) (Tailer, error) {
	return nil, errors.New("socket tailing is unavailable")
}

func (dug *dockerUtilGetterImpl) get() (container.DockerContainerLogInterface, error) {
	return nil, errors.New("docker log interface is unavailable")
}
