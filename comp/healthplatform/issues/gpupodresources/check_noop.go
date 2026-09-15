// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !kubelet

package gpupodresources

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

type checker struct{}

func newChecker(config.Component, hostnameinterface.Component) *checker {
	return &checker{}
}

func (*checker) Run() ([]runnerdef.IssueReport, error) {
	return nil, nil
}
