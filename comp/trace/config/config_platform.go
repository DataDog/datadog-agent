// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	traceconfigimpl "github.com/DataDog/datadog-agent/comp/trace/config/impl"
)

// DefaultLogFilePath is where the agent will write logs if not overridden in the conf.
// Deprecated: Use comp/trace/config/impl.DefaultLogFilePath directly.
var DefaultLogFilePath = traceconfigimpl.DefaultLogFilePath
