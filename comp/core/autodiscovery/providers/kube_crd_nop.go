// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build !(clusterchecks && kubeapiserver)

package providers

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// NewKubeCRDConfigProvider returns a new ConfigProvider connected to apiserver for CRDs.
var NewKubeCRDConfigProvider func(_ *pkgconfigsetup.ConfigurationProviders, _ *telemetry.Store) (types.ConfigProvider, error)
