package command
import (
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "path/filepath"
    "os"
    "testing"
    "github.com/spf13/cobra"
)


// Test generated using Keploy
func TestGetDefaultCoreBundleParams_ValidGlobalParams_ReturnsExpectedBundleParams(t *testing.T) {
    globalParams := &GlobalParams{
        ConfFilePath:        "/path/to/config",
        ExtraConfFilePath:   []string{"/path/to/extra/config1", "/path/to/extra/config2"},
        FleetPoliciesDirPath: "/path/to/fleet/policies",
    }

    bundleParams := GetDefaultCoreBundleParams(globalParams)

    assert.Equal(t, globalParams.ConfFilePath, bundleParams.ConfigParams.ConfFilePath, "ConfFilePath mismatch")
    assert.Len(t, bundleParams.ConfigParams.ExtraConfFilePath, len(globalParams.ExtraConfFilePath), "ExtraConfFilePath length mismatch")
    assert.Equal(t, globalParams.FleetPoliciesDirPath, bundleParams.ConfigParams.FleetPoliciesDirPath, "FleetPoliciesDirPath mismatch")
}

// Test generated using Keploy
func TestLogLevelDefaultOff_RegisterAndValue_ReturnsExpectedValue(t *testing.T) {
    logLevel := &LogLevelDefaultOff{}
    cmd := &cobra.Command{}

    logLevel.Register(cmd)

    flag := cmd.PersistentFlags().Lookup("log_level")
    require.NotNil(t, flag, "Expected 'log_level' flag to be registered")
    assert.Equal(t, "off", flag.DefValue, "Default log_level value mismatch")

    require.NoError(t, cmd.PersistentFlags().Set("log_level", "debug"))
    assert.Equal(t, "debug", logLevel.Value(), "Log level value mismatch")
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

    assert.Equal(t, filepath.Base(os.Args[0]), rootCmd.Use, "Root command Use mismatch")
    require.NotNil(t, rootCmd.PersistentFlags().Lookup("cfgpath"), "Expected persistent flag 'cfgpath' to be defined")
    
    assert.Len(t, rootCmd.Commands(), 2, "Subcommands count mismatch")
    assert.Equal(t, "subcommand1", rootCmd.Commands()[0].Use, "First subcommand mismatch")
    assert.Equal(t, "subcommand2", rootCmd.Commands()[1].Use, "Second subcommand mismatch")
}

