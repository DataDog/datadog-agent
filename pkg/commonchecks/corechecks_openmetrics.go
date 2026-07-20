// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build openmetrics && !(clusterchecks && kubeapiserver)

package commonchecks

import (
	corecheckLoader "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/openmetrics"
)

func registerOpenMetricsCheck() {
	corecheckLoader.RegisterCheck(openmetrics.CheckName, openmetrics.Factory())
}
