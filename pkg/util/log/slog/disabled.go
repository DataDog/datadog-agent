// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package slog

import (
	"github.com/DataDog/datadog-agent/pkg/util/log/slog/handlers"
	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

// Disabled returns a disabled logger.
func Disabled() types.LoggerInterface {
	disabledHandler := handlers.NewDisabled()
	return NewWrapper(disabledHandler)
}
