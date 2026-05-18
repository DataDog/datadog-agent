// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !ncm

// Package networkconfigmanagementimpl implements a stub component when ncm is disabled.
package networkconfigmanagementimpl

import (
	compdef "github.com/DataDog/datadog-agent/comp/def"
	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// NewComponent creates a stub networkconfigmanagement component
func NewComponent(_ compdef.In) (Provides, error) {
	provides := Provides{
		Comp:              option.None[networkconfigmanagement.Component](),
		GetConfigEndpoint: nilEndpoint(),
	}
	return provides, nil
}
