// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package kubernetes

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
)

func NewDecoder(source *config.LogSource) *decoder.GDecoder {
	return decoder.NewDecoderWithSource(source, &decoder.NewLineMatcher{}, &Convertor{})
}
