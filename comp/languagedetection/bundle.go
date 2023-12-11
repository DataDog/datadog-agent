// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package languagedetection implements the "languagedetection" bundle
package languagedetection

import (
	"github.com/DataDog/datadog-agent/comp/languagedetection/client/clientimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: container-integrations

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		clientimpl.Module())
}

// MockBundle defines the fx options for this bundle.
func MockBundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		clientimpl.Module())
}
