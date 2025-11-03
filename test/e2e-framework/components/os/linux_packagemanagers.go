// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package os

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func newAptManager(runner command.Runner) PackageManager {
	return NewGenericPackageManager(runner, "apt", "apt-get install -y", "apt-get update -y", "apt-get remove -y",
		pulumi.StringMap{"DEBIAN_FRONTEND": pulumi.String("noninteractive")})
}

func newYumManager(runner command.Runner) PackageManager {
	return NewGenericPackageManager(runner, "yum", "yum install -y", "", "yum remove -y", nil)
}

func newDnfManager(runner command.Runner) PackageManager {
	return NewGenericPackageManager(runner, "dnf", "dnf install -y", "", "dnf remove -y", nil)
}
