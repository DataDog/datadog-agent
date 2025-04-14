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

// func (i *InstallerExec) RunHook(ctx context.Context, pkg string, hook string, packageType string, upgrade bool, windowsArgs []string) (err error) {
// 	serializedWindowsArgs, err := json.Marshal(windowsArgs)
// 	cmd := i.newInstallerCmd(ctx, "hooks", hook, pkg, packageType, strconv.FormatBool(upgrade), string(serializedWindowsArgs))

func hooksCommand() *cobra.Command {
	return &cobra.Command{
		Hidden:             true,
		Use:                "hooks <hook> <package> <type:deb|rpm|oci> <upgrade:true|false> <windowsArgs:[json]>",
		Short:              "Run hooks for a package",
		GroupID:            "installer",
		DisableFlagParsing: true,
		Args:               cobra.MinimumNArgs(5),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i := newCmd(fmt.Sprintf("hooks.%s.%s", args[1], args[0]))
			defer i.stop(err)
			hook := args[0]
			pkg := args[1]
			rawPackageType := args[2]
			packageType, err := parsePackageType(rawPackageType)
			if err != nil {
				return err
			}
			upgrade := args[3] == "true"
			var windowsArgs []string
			err = json.Unmarshal([]byte(args[4]), &windowsArgs)
			if err != nil {
				return err
			}
			return packages.RunHook(i.ctx, pkg, hook, packageType, upgrade, windowsArgs)
		},
	}
}

func postinstCommand() *cobra.Command {
	return &cobra.Command{
		Hidden:  true,
		Use:     "postinst <package> <type:deb|rpm|oci>",
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
			return packages.RunHook(i.ctx, pkg, "postinst", packageType, false, nil)
		},
	}
}

func parsePackageType(rawPackageType string) (packages.PackageType, error) {
	switch rawPackageType {
	case string(packages.PackageTypeDEB):
		return packages.PackageTypeDEB, nil
	case string(packages.PackageTypeRPM):
		return packages.PackageTypeRPM, nil
	case string(packages.PackageTypeOCI):
		return packages.PackageTypeOCI, nil
	default:
		return "", fmt.Errorf("unknown package type: %s", rawPackageType)
	}
}
