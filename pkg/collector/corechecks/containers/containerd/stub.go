// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build !containerd

//nolint:revive // TODO(CINT) Fix revive linter
package containerd

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "containerd"
)

// Factory is used to create register the check and initialize it.
func Factory(workloadmeta.Component, workloadfilter.Component, tagger.Component) option.Option[func() check.Check] {
	return option.None[func() check.Check]()
}
