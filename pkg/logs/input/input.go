// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package input

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

type Input interface {
	Add(source *config.LogSource)
	Remove(source *config.LogSource)
}
