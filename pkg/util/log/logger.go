// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

type LoggerInterface = types.LoggerInterface

// Default returns a default logger
func Default() LoggerInterface {
	return seelog.Default
}

// Disabled returns a disabled logger
func Disabled() LoggerInterface {
	return seelog.Disabled
}
