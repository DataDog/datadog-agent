// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !cel || !servicenaming

package init

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// initServiceNaming is a no-op when the 'cel' build tag is not set.
// This prevents CEL dependencies from being pulled into builds that don't need them.
func initServiceNaming(_ context.Context, _ workloadmeta.Component, _ config.Component) error {
	return nil
}
