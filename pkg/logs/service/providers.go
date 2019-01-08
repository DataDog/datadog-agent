// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package service

import (
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// Service providers
const (
	Docker     = containers.RuntimeNameDocker
	Containerd = containers.RuntimeNameContainerd
)
