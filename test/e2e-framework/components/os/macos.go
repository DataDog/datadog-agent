// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package os

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
)

func newMacOS(e config.Env, desc Descriptor, runner command.Runner) OS {
	os := &os{
		descriptor:     desc,
		runner:         runner,
		fileManager:    command.NewFileManager(runner),
		packageManager: newBrewManager(runner),
		serviceManager: newMacOSServiceManager(e, runner),
	}

	return os
}
