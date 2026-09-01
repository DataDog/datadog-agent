// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/spf13/cobra"
)

func isPrermSupportedCommand() *cobra.Command {
	return &cobra.Command{
		Hidden: true,
		Use:    "is-prerm-supported",
		Short:  "Check if prerm is supported",
		Run: func(_ *cobra.Command, _ []string) {
			os.Exit(0)
		},
	}
}

func hooksCommand() *cobra.Command {
	return &cobra.Command{
		Hidden:             true,
		Use:                "hooks <hookContext>",
		Short:              "Run hooks for a package",
		GroupID:            "installer",
		DisableFlagParsing: true,
		Args:               cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i := newCmd("hooks")
			defer i.stop(err)
			var hookContext packages.HookContext
			err = json.Unmarshal([]byte(args[0]), &hookContext)
			if err != nil {
				return err
			}
			hookContext.Context = i.ctx
			return packages.RunHook(hookContext)
		},
	}
}

func postinstCommand() *cobra.Command {
	return &cobra.Command{
		Hidden:  true,
		Use:     "postinst <package> <type:deb|rpm|dmg>",
		Short:   "Run post-install scripts for a package",
		GroupID: "installer",
		Args:    cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i := newCmd("postinst")
			defer i.stop(err)
			pkg := args[0]
			rawPackageType := args[1]
			packageType, err := parsePackageType(rawPackageType)
			if err != nil {
				return err
			}
			packagePath, err := postinstPackagePath(packageType)
			if err != nil {
				return err
			}
			hookContext := packages.HookContext{
				Context:     i.ctx,
				Hook:        "postInstall",
				Package:     pkg,
				PackagePath: packagePath,
				PackageType: packageType,
				Upgrade:     false,
				WindowsArgs: nil,
			}
			return packages.RunHook(hookContext)
		},
	}
}

func prermCommand() *cobra.Command {
	upgrade := false
	c := &cobra.Command{
		Hidden:  true,
		Use:     "prerm <package> <type:deb|rpm|dmg>",
		Short:   "Run pre-remove scripts for a package",
		GroupID: "installer",
		Args:    cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i := newCmd("prerm")
			defer i.stop(err)
			pkg := args[0]
			rawPackageType := args[1]
			packageType, err := parsePackageType(rawPackageType)
			if err != nil {
				return err
			}
			packagePath, err := postinstPackagePath(packageType)
			if err != nil {
				return err
			}
			hookContext := packages.HookContext{
				Context:     i.ctx,
				Hook:        "preRemove",
				Package:     pkg,
				PackagePath: packagePath,
				PackageType: packageType,
				Upgrade:     upgrade,
				WindowsArgs: nil,
			}
			return packages.RunHook(hookContext)
		},
	}
	c.Flags().BoolVar(&upgrade, "upgrade", false, "Run the pre-remove script for an upgrade")
	return c
}

func parsePackageType(rawPackageType string) (packages.PackageType, error) {
	switch rawPackageType {
	case string(packages.PackageTypeMSI):
		return packages.PackageTypeMSI, nil
	case string(packages.PackageTypeDEB):
		return packages.PackageTypeDEB, nil
	case string(packages.PackageTypeRPM):
		return packages.PackageTypeRPM, nil
	case string(packages.PackageTypeDMG):
		return packages.PackageTypeDMG, nil
	default:
		return "", fmt.Errorf("unknown package type: %s", rawPackageType)
	}
}

// postinstPackagePath returns the PackagePath the postinst and prerm hooks run against.
//
// For deb and rpm the Agent lives at one fixed path. For a .dmg the payload is in the versioned
// pool and nothing has named it yet -- the stable link is created by the very hook this call is
// about to run -- so the version directory is recovered from the running binary's own path. The
// .dmg's post-install script execs
// /opt/datadog-packages/datadog-agent/<version>/embedded/bin/installer, so walking three
// directories up from the executable lands on the version directory. Deriving it rather than
// passing it as an argument means the script cannot name a version whose payload is not the one
// running.
func postinstPackagePath(packageType packages.PackageType) (string, error) {
	if packageType != packages.PackageTypeDMG {
		return "/opt/datadog-agent", nil
	}
	executable, err := exec.GetExecutable()
	if err != nil {
		return "", fmt.Errorf("could not locate the running installer: %w", err)
	}
	// <version>/embedded/bin/installer -> <version>
	versionDir := filepath.Dir(filepath.Dir(filepath.Dir(executable)))
	expectedParent := filepath.Join(paths.PackagesPath, "datadog-agent")
	if filepath.Dir(versionDir) != expectedParent {
		return "", fmt.Errorf("the dmg package type expects the installer to run from %s/<version>/embedded/bin, but it is running from %s", expectedParent, executable)
	}
	return versionDir, nil
}
