// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
)

var (
	apmInjectPackage = hooks{
		postInstall: postInstallAPMInjector,
		preRemove:   preRemoveAPMInjector,
	}
)

// postInstallAPMInjector is called after the APM injector is installed
func postInstallAPMInjector(ctx HookContext) (err error) {
	span, _ := ctx.StartSpan("setup_injector")
	defer func() { span.Finish(err) }()

	ddInjectPath := "/usr/local/bin/dd-inject"
	if err := os.MkdirAll("/usr/local/bin", 0755); err != nil {
		return err
	}
	err = os.WriteFile(ddInjectPath, embedded.ScriptDDInject, 0755)
	if err != nil {
		return err
	}
	return os.Chmod(ddInjectPath, 0755)
}

// preRemoveAPMInjector is called before the APM injector is removed
func preRemoveAPMInjector(ctx HookContext) (err error) {
	span, _ := ctx.StartSpan("remove_injector")
	defer func() { span.Finish(err) }()

	return os.Remove("/usr/local/bin/dd-inject")
}
