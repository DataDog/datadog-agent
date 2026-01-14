// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logslibrary provides the logs component bundle
package logslibrary

import (
	auditorfx "github.com/DataDog/datadog-agent/comp/logs-library/auditor/fx"
	kubehealthfx "github.com/DataDog/datadog-agent/comp/logs-library/kubehealth/fx"
	validatorfx "github.com/DataDog/datadog-agent/comp/logs-library/validator/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-log-pipelines

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		validatorfx.Module(),
		kubehealthfx.Module(),
		auditorfx.Module(),
	)
}

// Disabled Variant must be included for all api based components, with them set to None
