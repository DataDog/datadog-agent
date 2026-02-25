// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flare implements 'otel-agent flare'.
package flare

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpsprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.uber.org/fx"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	extensiontypes "github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/input"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*subcommands.GlobalParams

	// args are the positional command-line arguments
	args []string

	// subcommand-specific flags
	customerEmail string
	autoconfirm   bool
}

// MakeCommand returns the flare subcommand for the 'otel-agent' command.
func MakeCommand(globalConfGetter func() *subcommands.GlobalParams) *cobra.Command {
	cliParams := &cliParams{}

	flareCmd := &cobra.Command{
		Use:   "flare [caseID]",
		Short: "Collect a flare and send it to Datadog",
		Long:  `Collects diagnostic information from the OTel Agent and sends it to Datadog support`,
		RunE: func(_ *cobra.Command, args []string) error {
			globalParams := globalConfGetter()
			cliParams.GlobalParams = globalParams
			cliParams.args = args

			return fxutil.OneShot(makeFlare,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams("", config.WithConfigName(globalParams.ConfigName)),
					LogParams:    log.ForOneShot(globalParams.LoggerName, "info", false),
				}),
				flare.Module(flare.NewLocalParams(
					"", // distPath - not used for OTel Agent
					"", // pyChecksPath - not used for OTel Agent
					"", // logFilePath - not used for OTel Agent
					"", // jmxLogFilePath - not used for OTel Agent
					"", // dogstatsDLogFilePath - not used for OTel Agent
					"", // streamlogsLogFilePath - not used for OTel Agent
				)),
				core.Bundle(core.WithSecrets()),
				// Provide empty option for workloadmeta (optional dependency)
				fx.Supply(option.None[workloadmeta.Component]()),
				// Provide required modules
				ipcfx.ModuleInsecure(),
			)
		},
	}

	flareCmd.Flags().StringVarP(&cliParams.customerEmail, "email", "e", "", "Your email")
	flareCmd.Flags().BoolVarP(&cliParams.autoconfirm, "send", "s", false, "Automatically send flare (don't prompt for confirmation)")
	flareCmd.SetArgs([]string{"caseID"})

	return flareCmd
}

func makeFlare(
	flareComp flare.Component,
	_ log.Component,
	_ config.Component,
	cliParams *cliParams,
) error {
	// Get case ID
	caseID := ""
	if len(cliParams.args) > 0 {
		caseID = cliParams.args[0]
	}

	// Get customer email
	customerEmail := cliParams.customerEmail
	if customerEmail == "" {
		var err error
		customerEmail, err = input.AskForEmail()
		if err != nil {
			fmt.Println("Error reading email, please retry or contact support")
			return err
		}
	}

	// Collect flare data
	fmt.Fprintln(color.Output, color.BlueString("Collecting diagnostic data from OTel Agent configuration..."))
	filePath, err := createOTelFlare(cliParams.GlobalParams)
	if err != nil {
		return err
	}

	// Check if file exists
	if _, err := os.Stat(filePath); err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("The flare zipfile \"%s\" does not exist.", filePath)))
		return err
	}

	// Confirm upload
	fmt.Fprintf(color.Output, "%s is going to be uploaded to Datadog\n", color.YellowString(filePath))
	if !cliParams.autoconfirm {
		confirmation := input.AskForConfirmation("Are you sure you want to upload a flare? [y/N]")
		if !confirmation {
			fmt.Fprintf(color.Output, "Aborting. (You can still use %s)\n", color.YellowString(filePath))
			return nil
		}
	}

	// Upload flare
	response, e := flareComp.Send(filePath, caseID, customerEmail, helpers.NewLocalFlareSource())
	fmt.Println(response)
	if e != nil {
		return e
	}
	return nil
}

// createOTelFlare collects diagnostic data and creates a flare archive
func createOTelFlare(params *subcommands.GlobalParams) (string, error) {
	// Collect data
	fmt.Fprintln(color.Output, "Collecting OTel configuration data...")
	data, err := collectOTelData(params)
	if err != nil {
		return "", fmt.Errorf("failed to collect OTel data: %w", err)
	}

	// Create flare archive
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("otel-agent-flare_%s.zip", timestamp)
	filePath := filepath.Join(os.TempDir(), filename)

	err = createFlareArchive(filePath, data)
	if err != nil {
		return "", fmt.Errorf("failed to create flare archive: %w", err)
	}

	fmt.Fprintln(color.Output, color.GreenString("Flare archive created: "+filePath))
	return filePath, nil
}

// collectOTelData collects diagnostic data similar to ddflareextension
func collectOTelData(params *subcommands.GlobalParams) (*extensiontypes.Response, error) {
	ctx := context.Background()

	// Build URIs from configuration paths
	uris := make([]string, len(params.ConfPaths))
	copy(uris, params.ConfPaths)

	// Add sets as YAML URIs
	uris = append(uris, params.Sets...)

	var customerConfig, runtimeConfig, envConfig string
	var err error

	if len(uris) > 0 {
		// Load customer-provided configuration
		customerConfig, err = loadCustomerConfig(ctx, uris)
		if err != nil {
			fmt.Fprintf(color.Output, "%s", color.YellowString(fmt.Sprintf("Warning: Could not load customer config: %v\n", err)))
		}

		// Load runtime configuration with all providers
		runtimeConfig, envConfig, err = loadRuntimeConfig(ctx, uris)
		if err != nil {
			fmt.Fprintf(color.Output, "%s", color.YellowString(fmt.Sprintf("Warning: Could not load runtime config: %v\n", err)))
		}
	} else {
		customerConfig = "# No configuration files provided"
		runtimeConfig = "# No configuration files provided"
		envConfig = "# No configuration files provided"
	}

	// Collect environment variables
	envVars := getEnvironmentAsMap()

	// Discover debug source URLs from runtime configuration
	debugSources := discoverDebugSources(ctx, uris)

	// Build response
	resp := &extensiontypes.Response{
		BuildInfoResponse: extensiontypes.BuildInfoResponse{
			AgentVersion:     version.AgentVersion,
			AgentCommand:     "otel-agent",
			AgentDesc:        "Datadog OTel Agent",
			ExtensionVersion: "flare-command",
			BYOC:             params.BYOC,
		},
		ConfigResponse: extensiontypes.ConfigResponse{
			CustomerConfig:        customerConfig,
			RuntimeConfig:         runtimeConfig,
			RuntimeOverrideConfig: "", // TODO: support RemoteConfig
			EnvConfig:             envConfig,
		},
		DebugSourceResponse: extensiontypes.DebugSourceResponse{
			Sources: debugSources,
		},
		Environment: envVars,
	}

	return resp, nil
}

// loadCustomerConfig loads the customer-provided configuration
func loadCustomerConfig(ctx context.Context, uris []string) (string, error) {
	// Use only file provider to get raw customer config
	rs := confmap.ResolverSettings{
		URIs: uris,
		ProviderFactories: []confmap.ProviderFactory{
			fileprovider.NewFactory(),
			yamlprovider.NewFactory(),
		},
		DefaultScheme: "file",
	}

	resolver, err := confmap.NewResolver(rs)
	if err != nil {
		return "", err
	}

	cfg, err := resolver.Resolve(ctx)
	if err != nil {
		return "", err
	}

	confMap := cfg.ToStringMap()
	yamlBytes, err := yaml.Marshal(confMap)
	if err != nil {
		return "", err
	}

	return string(yamlBytes), nil
}

// loadRuntimeConfig loads the full runtime configuration with all providers
func loadRuntimeConfig(ctx context.Context, uris []string) (string, string, error) {
	rs := confmap.ResolverSettings{
		URIs: uris,
		ProviderFactories: []confmap.ProviderFactory{
			fileprovider.NewFactory(),
			envprovider.NewFactory(),
			yamlprovider.NewFactory(),
			httpprovider.NewFactory(),
			httpsprovider.NewFactory(),
		},
		DefaultScheme: "env",
	}

	resolver, err := confmap.NewResolver(rs)
	if err != nil {
		return "", "", err
	}

	cfg, err := resolver.Resolve(ctx)
	if err != nil {
		return "", "", err
	}

	confMap := cfg.ToStringMap()

	// Create runtime config with all substitutions
	runtimeBytes, err := yaml.Marshal(confMap)
	if err != nil {
		return "", "", err
	}

	// Create env config by replacing environment variable values with placeholders
	envConfMap := replaceEnvVarsWithPlaceholders(confMap)
	envBytes, err := yaml.Marshal(envConfMap)
	if err != nil {
		return "", "", err
	}

	return string(runtimeBytes), string(envBytes), nil
}

// replaceEnvVarsWithPlaceholders replaces environment variable values with ${env:VAR_NAME} placeholders
func replaceEnvVarsWithPlaceholders(confMap map[string]any) map[string]any {
	result := make(map[string]any)

	// Get all environment variables
	envVars := getEnvironmentAsMap()

	for key, value := range confMap {
		result[key] = replaceValueWithEnvPlaceholder(value, envVars)
	}

	return result
}

// replaceValueWithEnvPlaceholder recursively replaces values that match environment variables
func replaceValueWithEnvPlaceholder(value any, envVars map[string]string) any {
	switch v := value.(type) {
	case string:
		// Check if this string value matches any environment variable
		for envKey, envVal := range envVars {
			if v == envVal && envVal != "" {
				return fmt.Sprintf("${env:%s}", envKey)
			}
		}
		return v
	case map[string]any:
		result := make(map[string]any)
		for k, val := range v {
			result[k] = replaceValueWithEnvPlaceholder(val, envVars)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, val := range v {
			result[i] = replaceValueWithEnvPlaceholder(val, envVars)
		}
		return result
	default:
		return v
	}
}

// getEnvironmentAsMap returns all environment variables as a map
func getEnvironmentAsMap() map[string]string {
	env := map[string]string{}
	for _, keyVal := range os.Environ() {
		split := strings.SplitN(keyVal, "=", 2)
		if len(split) == 2 {
			key, val := split[0], split[1]
			env[key] = val
		}
	}
	return env
}

// discoverDebugSources discovers debug extension URLs from the configuration
func discoverDebugSources(ctx context.Context, uris []string) map[string]extensiontypes.OTelFlareSource {
	sources := make(map[string]extensiontypes.OTelFlareSource)

	if len(uris) == 0 {
		return sources
	}

	// Load runtime configuration to discover extensions
	rs := confmap.ResolverSettings{
		URIs: uris,
		ProviderFactories: []confmap.ProviderFactory{
			fileprovider.NewFactory(),
			envprovider.NewFactory(),
			yamlprovider.NewFactory(),
			httpprovider.NewFactory(),
			httpsprovider.NewFactory(),
		},
		DefaultScheme: "env",
	}

	resolver, err := confmap.NewResolver(rs)
	if err != nil {
		return sources
	}

	cfg, err := resolver.Resolve(ctx)
	if err != nil {
		return sources
	}

	// Look for extensions in the configuration
	extensionsConf, err := cfg.Sub("extensions")
	if err != nil {
		return sources
	}

	extensions := extensionsConf.ToStringMap()
	for extensionName := range extensions {
		// Extract extension type (e.g., "pprof/dd-autoconfigured" -> "pprof")
		extType := extractExtensionType(extensionName)

		// Check if this is a supported debug extension
		extractor, ok := supportedDebugExtensions[extType]
		if !ok {
			continue
		}

		// Get the extension configuration
		exconf, err := extensionsConf.Sub(extensionName)
		if err != nil {
			continue
		}

		// Extract the endpoint URL
		uri, err := extractor(exconf)
		if err != nil {
			continue
		}

		// Generate URLs based on extension type
		var uris []string
		switch extType {
		case "pprof":
			uris = []string{
				uri + "/debug/pprof/heap",
				uri + "/debug/pprof/allocs",
				uri + "/debug/pprof/profile",
			}
		case "zpages":
			uris = []string{
				uri + "/debug/servicez",
				uri + "/debug/pipelinez",
				uri + "/debug/extensionz",
				uri + "/debug/featurez",
				uri + "/debug/tracez",
			}
		default:
			uris = []string{uri}
		}

		sources[extensionName] = extensiontypes.OTelFlareSource{
			URLs: uris,
		}
	}

	return sources
}

// extractExtensionType extracts the extension type from a full extension name
// e.g., "pprof/dd-autoconfigured" -> "pprof"
func extractExtensionType(extensionName string) string {
	if idx := strings.Index(extensionName, "/"); idx != -1 {
		return extensionName[:idx]
	}
	return extensionName
}

// supportedDebugExtensions maps extension types to their endpoint extractors
var supportedDebugExtensions = map[string]func(*confmap.Conf) (string, error){
	"health_check": extractHealthCheckEndpoint,
	"pprof":        extractPprofEndpoint,
	"zpages":       extractZpagesEndpoint,
}

// extractHealthCheckEndpoint extracts the endpoint from a health_check extension config
func extractHealthCheckEndpoint(conf *confmap.Conf) (string, error) {
	endpoint := "localhost:13133" // default
	if conf.IsSet("endpoint") {
		endpoint = conf.Get("endpoint").(string)
	}
	return "http://" + endpoint, nil
}

// extractPprofEndpoint extracts the endpoint from a pprof extension config
func extractPprofEndpoint(conf *confmap.Conf) (string, error) {
	endpoint := "localhost:1777" // default
	if conf.IsSet("endpoint") {
		endpoint = conf.Get("endpoint").(string)
	}
	return "http://" + endpoint, nil
}

// extractZpagesEndpoint extracts the endpoint from a zpages extension config
func extractZpagesEndpoint(conf *confmap.Conf) (string, error) {
	endpoint := "localhost:55679" // default
	if conf.IsSet("endpoint") {
		endpoint = conf.Get("endpoint").(string)
	}
	return "http://" + endpoint, nil
}

// createFlareArchive creates a zip archive with the diagnostic data
func createFlareArchive(filePath string, data *extensiontypes.Response) error {
	// Create zip file
	zipFile, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Add raw response JSON
	rawJSON, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}
	if err := addFileToZip(zipWriter, "otel-response.json", rawJSON); err != nil {
		return err
	}

	// Add build info
	buildInfo := fmt.Sprintf("Agent Version: %s\nAgent Command: %s\nAgent Description: %s\nExtension Version: %s\nBYOC: %v\n",
		data.AgentVersion, data.AgentCommand, data.AgentDesc, data.ExtensionVersion, data.BYOC)
	if err := addFileToZip(zipWriter, "build-info.txt", []byte(buildInfo)); err != nil {
		return err
	}

	// Add configurations
	if data.CustomerConfig != "" {
		if err := addFileToZip(zipWriter, "config/customer-config.yaml", []byte(data.CustomerConfig)); err != nil {
			return err
		}
	}
	if data.EnvConfig != "" {
		if err := addFileToZip(zipWriter, "config/env-config.yaml", []byte(data.EnvConfig)); err != nil {
			return err
		}
	}
	if data.RuntimeOverrideConfig != "" {
		if err := addFileToZip(zipWriter, "config/runtime-override-config.yaml", []byte(data.RuntimeOverrideConfig)); err != nil {
			return err
		}
	}
	if data.RuntimeConfig != "" {
		if err := addFileToZip(zipWriter, "config/runtime-config.yaml", []byte(data.RuntimeConfig)); err != nil {
			return err
		}
	}

	// Add environment variables
	if len(data.Environment) > 0 {
		envJSON, err := json.MarshalIndent(data.Environment, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal environment: %w", err)
		}
		if err := addFileToZip(zipWriter, "environment.json", envJSON); err != nil {
			return err
		}
	}

	// Add debug source URLs (if any)
	if len(data.Sources) > 0 {
		sourcesJSON, err := json.MarshalIndent(data.Sources, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal debug sources: %w", err)
		}
		if err := addFileToZip(zipWriter, "debug-sources.json", sourcesJSON); err != nil {
			return err
		}
	}

	return nil
}

// addFileToZip adds a file to the zip archive
func addFileToZip(zipWriter *zip.Writer, filename string, content []byte) error {
	writer, err := zipWriter.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file in zip: %w", err)
	}

	_, err = io.Copy(writer, bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("failed to write file to zip: %w", err)
	}

	return nil
}
