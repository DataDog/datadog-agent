// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck

package productcomposition

import (
	"go.uber.org/fx"

	sharedlibraryfx "github.com/DataDog/datadog-agent/comp/collector/sharedlibrary/fx-sharedlibrary"
)

func sharedLibraryCollectorOptions() []fx.Option {
	return []fx.Option{
		sharedlibraryfx.Module(),
	}
}

func sharedLibraryCheckOptions() []fx.Option {
	return []fx.Option{
		sharedlibraryfx.Module(),
	}
}
