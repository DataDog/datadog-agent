// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !ncm

package check

import (
	"go.uber.org/fx"

	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func getNCMModule(_ []string) fx.Option {
	return fx.Supply(option.None[networkconfigmanagement.Component]())
}
