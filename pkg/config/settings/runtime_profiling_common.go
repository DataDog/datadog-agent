// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/util/profiling"
)

func checkProfilingNeedsRestart(old, new int) error {
	if old == 0 && new != 0 && profiling.IsRunning() {
		return errors.New("go runtime setting applied; manually restart internal profiling to capture profile data in the app")
	}
	return nil
}
