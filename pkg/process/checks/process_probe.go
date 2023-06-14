// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package checks

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func newProcessProbe(config config.ConfigReader, options ...procutil.Option) procutil.Probe {
	return procutil.NewProcessProbe(options...)
}
