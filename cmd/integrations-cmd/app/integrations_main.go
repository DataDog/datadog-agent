// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build cpython

package app

import (
	"github.com/spf13/cobra"
)

const (
	tufConfigFile       = "public-tuf-config.json"
	reqAgentReleaseFile = "requirements-agent-release.txt"
	tufPkgPattern       = "datadog(-|_).*"
	tufIndex            = "https://dd-integrations-core-wheels-build-stable.s3.amazonaws.com/targets/simple/"
	reqLinePattern      = "%s==(\\d+\\.\\d+\\.\\d+)"
	yamlFilePattern     = "[\\w_]+\\.yaml.*"
)

var (
	allowRoot    bool
	withoutTuf   bool
	inToto       bool
	verbose      bool
	useSysPython bool
	tufConfig    string
)

type integrationVersion struct {
	major int
	minor int
	fix   int
}

func init() {
	AgentCmd.AddCommand(TufCmd)
	TufCmd.AddCommand(installCmd)
	TufCmd.AddCommand(removeCmd)
	TufCmd.AddCommand(searchCmd)
	TufCmd.AddCommand(freezeCmd)
	TufCmd.PersistentFlags().BoolVarP(&withoutTuf, "no-tuf", "t", false, "don't use TUF repo")
	TufCmd.PersistentFlags().BoolVarP(&inToto, "in-toto", "i", false, "enable in-toto")
	TufCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose logging on pip and TUF")
	TufCmd.PersistentFlags().BoolVarP(&allowRoot, "allow-root", "r", false, "flag to enable root to install packages")
	TufCmd.PersistentFlags().BoolVarP(&useSysPython, "use-sys-python", "p", false, "use system python instead [dev flag]")
	TufCmd.PersistentFlags().StringVar(&tufConfig, "tuf-cfg", getTufConfigPath(), "path to TUF config file")

	// Power user flags - mark as hidden
	TufCmd.PersistentFlags().MarkHidden("use-sys-python")
}

// TufCmd export as part of allowing us to share code with the separate exe
var TufCmd = &cobra.Command{
	Use:   "integration [command]",
	Short: "Datadog integration manager (ALPHA feature)",
	Long:  ``,
}

var installCmd = &cobra.Command{
	Use:   "install [package]",
	Short: "Install Datadog integration core/extra packages",
	Long: `Install Datadog integration core/extra packages
You must specify a version of the package to install using the syntax: <package>==<version>, with
 - <package> of the form datadog-<integration-name>
 - <version> of the form x.y.z`,
	RunE: installTuf,
}

var removeCmd = &cobra.Command{
	Use:   "remove [package]",
	Short: "Remove Datadog integration core/extra packages",
	Long:  ``,
	RunE:  removeTuf,
}

var searchCmd = &cobra.Command{
	Use:    "search [package]",
	Short:  "Search Datadog integration core/extra packages",
	Long:   ``,
	RunE:   searchTuf,
	Hidden: true,
}

var freezeCmd = &cobra.Command{
	Use:   "freeze",
	Short: "Freeze list of installed python packages",
	Long:  ``,
	RunE:  freeze,
}
