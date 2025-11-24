// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package os

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

var (
	brewPackageNameMapping = map[string]string{}
)

func newBrewManager(runner command.Runner) PackageManager {
	return NewGenericPackageManager(runner, "brew", "brew install -y", "brew update -y", "brew uninstall -y",
		pulumi.StringMap{"NONINTERACTIVE": pulumi.String("1")}, brewPackageNameMapping)
}
