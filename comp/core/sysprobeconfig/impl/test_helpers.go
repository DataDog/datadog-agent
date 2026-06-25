// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package sysprobeconfigimpl

import (
	sysprobeconfigdef "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

// NewTestComponent creates a sysprobeconfig component from pre-built config objects.
// Only for use by the mock package.
func NewTestComponent(conf model.Config, syscfg *sysconfigtypes.Config) sysprobeconfigdef.Component {
	return &cfg{Config: conf, syscfg: syscfg}
}
