// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build ncm

package check

import (
	"go.uber.org/fx"

	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
	networkconfigmanagementfx "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/fx"
	ncm "github.com/DataDog/datadog-agent/pkg/collector/corechecks/networkconfigmanagement"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func getNCMModule(args []string) fx.Option {
	if len(args) > 0 && args[0] == ncm.CheckName {
		return networkconfigmanagementfx.Module()
	}
	// Other checks must not open or contend for the NCM rollback database.
	return fx.Supply(option.None[networkconfigmanagement.Component]())
}
