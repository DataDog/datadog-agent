// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package productcomposition

import "go.uber.org/fx"

func collectorOptions() []fx.Option {
	return append(pythonCollectorOptions(), sharedLibraryCollectorOptions()...)
}

func checkOptions() []fx.Option {
	return append(pythonCheckOptions(), sharedLibraryCheckOptions()...)
}
