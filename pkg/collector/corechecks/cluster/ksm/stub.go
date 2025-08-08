// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build !kubeapiserver

// Package ksm implements the Kubernetes State Core cluster check.
package ksm

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "kubernetes_state_core"
)

// Factory creates a new check factory
func Factory(_ tagger.Component) option.Option[func() check.Check] {
	return option.None[func() check.Check]()
}
