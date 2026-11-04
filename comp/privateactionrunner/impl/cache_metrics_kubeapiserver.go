// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package privateactionrunnerimpl

import (
	"context"

	"github.com/DataDog/datadog-go/v5/statsd"
)

func (*PrivateActionRunner) startCacheMetrics(context.Context, statsd.ClientInterface) {}
