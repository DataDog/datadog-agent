// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/packagemanager"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	agentDDOTPackage      = "datadog-agent-ddot"
	datadogYamlPath       = "/etc/datadog-agent/datadog.yaml"
	otelConfigPath        = "/etc/datadog-agent/otel-config.yaml"
	otelConfigExamplePath = "/etc/datadog-agent/otel-config.yaml.example"
)

// ddotConfigPermissions are the ownerships and modes that are enforced on the DDOT configuration files for OCI packages
var ddotConfigPermissions = file.Permissions{
	{Path: "otel-config.yaml.example", Owner: "dd-agent", Group: "dd-agent", Mode: 0640},
	{Path: "otel-config.yaml", Owner: "dd-agent", Group: "dd-agent", Mode: 0640},
}

//////////////////////////////
/// DDOT EXTENSION METHODS ///
//////////////////////////////

// preInstallDDOTExtension stops and removes the existing DDOT service and package before extension installation
func preInstallDDOTExtension(ctx HookContext) error {
	span, ctx := ctx.StartSpan("pre_install_extension_ddot")
	defer span.Finish(nil)

	if err := packagemanager.RemovePackage(ctx, agentDDOTPackage); err != nil {
		log.Warnf("failed to remove deb/rpm package: %s", err)
	}
	return nil
}

// postInstallDDOTExtension is the post-install hook for the DDOT extension
func postInstallDDOTExtension(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("post_install_extension_ddot")
	defer func() {
		span.Finish(err)
	}()

	// extensionPath is the path to the DDOT extension. It is already scoped to stable / experiment per the Agent package.
	extensionPath := filepath.Join(ctx.PackagePath, "ext", "ddot")

	// Copy the example file to the configuration directory
	// XXX: Maybe we should always embed the example file in the Agent package?
	if err := copyFile(filepath.Join(extensionPath, otelConfigExamplePath), otelConfigExamplePath, 0640); err != nil {
		return fmt.Errorf("failed to copy otel-config.yaml.example to /etc/datadog-agent: %v", err)
	}

	// Write otel-config.yaml. Doesn't update the file if it already exists.
	if err := writeOTelConfigCommon(ctx, datadogYamlPath, otelConfigExamplePath, otelConfigPath, true, 0640); err != nil {
		return fmt.Errorf("failed to write otel-config.yaml: %w", err)
	}

	// Enable the DDOT IPC server in datadog.yaml
	if err := enableOTelCollectorConfigInDatadogYAML(ctx, datadogYamlPath); err != nil {
		return fmt.Errorf("failed to enable otelcollector in datadog.yaml: %v", err)
	}

	// Ensure the DDOT configuration files have the correct permissions
	if err = ddotConfigPermissions.Ensure(ctx, "/etc/datadog-agent"); err != nil {
		return fmt.Errorf("failed to set DDOT config ownerships: %v", err)
	}

	return nil
}

// preRemoveDDOTExtension stops and disables the DDOT service before extension removal
func preRemoveDDOTExtension(ctx HookContext) error {
	span, _ := ctx.StartSpan("pre_remove_extension_ddot")
	defer span.Finish(nil)

	// Disable the DDOT IPC server in datadog.yaml.
	// During an upgrade, this will be re-enabled by the post-install hook. This gives us flexibility to change the config during upgrade.
	if err := disableOtelCollectorConfigCommon(datadogYamlPath); err != nil {
		log.Warnf("failed to disable otelcollector config: %s", err)
	}

	return nil
}

// copyFile copies a file from src to dst with the specified permissions
func copyFile(src, dst string, perm os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, perm)
}
