package command

import (
    "testing"
    "github.com/spf13/cobra"
    "os"
    "path/filepath"
)


// Test generated using Keploy
func TestGetDefaultCoreBundleParams_ValidGlobalParams_ReturnsExpectedBundleParams(t *testing.T) {
    globalParams := &GlobalParams{
        ConfFilePath:        "/path/to/config",
        ExtraConfFilePath:   []string{"/path/to/extra/config1", "/path/to/extra/config2"},
        FleetPoliciesDirPath: "/path/to/fleet/policies",
    }

    bundleParams := GetDefaultCoreBundleParams(globalParams)

    if bundleParams.ConfigParams.ConfFilePath != globalParams.ConfFilePath {
        t.Errorf("Expected ConfFilePath to be %v, got %v", globalParams.ConfFilePath, bundleParams.ConfigParams.ConfFilePath)
    }

    if len(bundleParams.ConfigParams.ExtraConfFilePath) != len(globalParams.ExtraConfFilePath) {
        t.Errorf("Expected ExtraConfFilePath length to be %v, got %v", len(globalParams.ExtraConfFilePath), len(bundleParams.ConfigParams.ExtraConfFilePath))
    }

    if bundleParams.ConfigParams.FleetPoliciesDirPath != globalParams.FleetPoliciesDirPath {
        t.Errorf("Expected FleetPoliciesDirPath to be %v, got %v", globalParams.FleetPoliciesDirPath, bundleParams.ConfigParams.FleetPoliciesDirPath)
    }
}

// Test generated using Keploy
func TestLogLevelDefaultOff_RegisterAndValue_ReturnsExpectedValue(t *testing.T) {
    logLevel := &LogLevelDefaultOff{}
    cmd := &cobra.Command{}

    logLevel.Register(cmd)

    flag := cmd.PersistentFlags().Lookup("log_level")
    if flag == nil {
        t.Fatalf("Expected 'log_level' flag to be registered")
    }

    if flag.DefValue != "off" {
        t.Errorf("Expected default value of 'log_level' to be 'off', got %v", flag.DefValue)
    }

    cmd.PersistentFlags().Set("log_level", "debug")
    if logLevel.Value() != "debug" {
        t.Errorf("Expected log level value to be 'debug', got %v", logLevel.Value())
    }
}


// Test generated using Keploy
func TestMakeCommand_ValidSubcommandFactories_CreatesExpectedCommand(t *testing.T) {
    subcommandFactory := func(globalParams *GlobalParams) []*cobra.Command {
        return []*cobra.Command{
            {
                Use:   "subcommand1",
                Short: "This is subcommand1",
            },
            {
                Use:   "subcommand2",
                Short: "This is subcommand2",
            },
        }
    }

    rootCmd := MakeCommand([]SubcommandFactory{subcommandFactory})

    if rootCmd.Use != filepath.Base(os.Args[0]) {
        t.Errorf("Expected root command Use to be %v, got %v", filepath.Base(os.Args[0]), rootCmd.Use)
    }

    if rootCmd.PersistentFlags().Lookup("cfgpath") == nil {
        t.Errorf("Expected persistent flag 'cfgpath' to be defined")
    }

    if len(rootCmd.Commands()) != 2 {
        t.Errorf("Expected 2 subcommands, got %v", len(rootCmd.Commands()))
    }

    if rootCmd.Commands()[0].Use != "subcommand1" {
        t.Errorf("Expected first subcommand to be 'subcommand1', got %v", rootCmd.Commands()[0].Use)
    }

    if rootCmd.Commands()[1].Use != "subcommand2" {
        t.Errorf("Expected second subcommand to be 'subcommand2', got %v", rootCmd.Commands()[1].Use)
    }
}

