// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

//nolint:revive // TODO(CINT) Fix revive linter
package kata

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "kata_containers"
)

// Factory returns a no-op check factory on non-Linux platforms
func Factory(workloadmeta.Component, tagger.Component) option.Option[func() check.Check] {
	return option.None[func() check.Check]()
}
