// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverimpl

import (
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/server/internal/wire"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func parserOptions(cfg model.Reader) wire.Options {
	return wire.Options{
		InternerSize:    cfg.GetInt("dogstatsd_string_interner_size"),
		ReadTimestamps:  cfg.GetBool("dogstatsd_no_aggregation_pipeline"),
		OriginDetection: cfg.GetBool("dogstatsd_origin_detection_client"),
	}
}

func newParser(cfg model.Reader, pool *wire.Float64ListPool, workerNum int, wmeta option.Option[workloadmeta.Component], internerTelemetry *wire.InternerTelemetry) *wire.Parser {
	p := wire.NewParser(parserOptions(cfg), pool, workerNum, internerTelemetry)
	// Preserve eager discovery and the first caller's workload metadata selection.
	provider.GetProvider(wmeta)
	return p
}

func newFloat64ListPool(cfg model.Reader, component telemetry.Component) *wire.Float64ListPool {
	return wire.NewFloat64ListPool(utils.IsTelemetryEnabled(cfg), component)
}
