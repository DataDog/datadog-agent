// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This combination of build tags ensures that this file is only included Agents that are not the Cluster Agent
//go:build !(clusterchecks && kubeapiserver) && !systemprobechecks

// Package commonchecks contains shared checks for multiple agent components
package commonchecks

import tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"

func registerSystemProbeChecks(_ tagger.Component) {
	// nothing to do here, this build doesn't include system-probe based checks
}
