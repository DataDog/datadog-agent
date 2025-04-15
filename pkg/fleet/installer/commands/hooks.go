// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package commands

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages"
	"github.com/spf13/cobra"
)

func hookCommand() *cobra.Command {
	return &cobra.Command{
		Hidden:             true,
		Use:                "hook <hookContext>",
		Short:              "Run a hook for a package",
		GroupID:            "installer",
		DisableFlagParsing: true,
		Args:               cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i := newCmd("hook")
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

// hookVersionCommand returns the version of the hook interface.
func hookVersionCommand() *cobra.Command {
	return &cobra.Command{
		Hidden:             true,
		Use:                "hook-version",
		Short:              "Get the version of the hook interface",
		GroupID:            "installer",
		DisableFlagParsing: true,
		Args:               cobra.ExactArgs(0),
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println(packages.HooksVersion)
		},
	}
}

func postinstCommand() *cobra.Command {
	return &cobra.Command{
		Hidden:  true,
		Use:     "postinst <package> <type:deb|rpm>",
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
			hookContext := packages.HookContext{
				Context:     i.ctx,
				Hook:        "postInstall",
				Package:     pkg,
				PackagePath: "/opt/datadog-agent",
				PackageType: packageType,
				Upgrade:     false,
				WindowsArgs: nil,
			}
			return packages.RunHook(hookContext)
		},
	}
}

func parsePackageType(rawPackageType string) (packages.PackageType, error) {
	switch rawPackageType {
	case string(packages.PackageTypeDEB):
		return packages.PackageTypeDEB, nil
	case string(packages.PackageTypeRPM):
		return packages.PackageTypeRPM, nil
	default:
		return "", fmt.Errorf("unknown package type: %s", rawPackageType)
	}
}
