// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !cri || !containerd

package configfilesdiscoveryimpl

import (
	"errors"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func newKubernetesConfigReader(target, workloadmeta.Component) (ConfigReader, error) {
	return nil, errors.New("no config reader for runtime")
}
