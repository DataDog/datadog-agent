// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package sender exposes a Sender for netflow.
package sender

import "github.com/DataDog/datadog-agent/comp/ndmtmp/aggregator"

func getDefaultSender(agg aggregator.Component) (Component, error) {
	return agg.GetDefaultSender()
}
