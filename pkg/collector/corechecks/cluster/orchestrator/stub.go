// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build !kubeapiserver || !orchestrator

//nolint:revive // TODO(CAPP) Fix revive linter
package orchestrator

import (
	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	// CheckName is the name of the check
	CheckName = "orchestrator"
)

// Factory creates a new check factory
func Factory(workloadmeta.Component, configcomp.Component, tagger.Component) optional.Option[func() check.Check] {
	return optional.NewNoneOption[func() check.Check]()
}
