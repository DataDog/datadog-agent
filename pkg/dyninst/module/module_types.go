// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
)

type procRuntimeID struct {
	process.ID
	service       string
	version       string
	environment   string
	runtimeID     string
	gitInfo       *process.GitInfo
	containerInfo *process.ContainerInfo
}
