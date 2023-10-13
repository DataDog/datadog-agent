// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/prometheus/common/model"
)

// metricTransformerFunc is used to tweak or generate new metrics from a given containerd metric
type metricTransformerFunc = func(sender.Sender, string, model.Sample)

var defaultContainerdOpenmetricsTransformers = map[string]metricTransformerFunc{
	"grpc_server_handled_total": grpcServerHandlerTransformer,
}

func grpcServerHandlerTransformer(s sender.Sender, name string, sample model.Sample) {
	metric := sample.Metric

	grpcMethod, ok := metric["grpc_method"]
	if !ok {
		return
	}

	switch grpcMethod {
	case pullImageGrpcMethod:
		imagePullMetricTransformer(s, name, sample)
	}
}

func imagePullMetricTransformer(s sender.Sender, name string, sample model.Sample) {
	metric := sample.Metric

	grpcCode, ok := metric["grpc_code"]

	if !ok {
		return
	}

	metricTags := []string{
		fmt.Sprintf("grpc_service:%s", metric["grpc_service"]),
		fmt.Sprintf("grpc_code:%s", toSnakeCase(string(grpcCode))),
	}

	s.MonotonicCount("containerd.image.pull", float64(sample.Value), "", metricTags)
}
