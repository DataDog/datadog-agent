// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package rcclient

import (
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

func (rc rcClient) SubscribeApmTracing() {
	pkglog.Info("APM TRACING config product is not supported outside Linux currently.")
}
