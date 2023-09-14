// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package npm for all NPM new E2E tests
package npm

import (
	_ "embed"
)

// SystemProbeConfig define the embedded minimal configuration for NPM
//
//go:embed config/npm.yaml
var SystemProbeConfig string
