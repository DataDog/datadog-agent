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
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
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
	"go.yaml.in/yaml/v3"

	agentConfig "github.com/DataDog/datadog-agent/cmd/otel-agent/config"
	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/comp/core/config"
	delegatedauthnoopfx "github.com/DataDog/datadog-agent/comp/core/delegatedauth/fx-noop"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logfx "github.com/DataDog/datadog-agent/comp/core/log/fx"
	schemanoopfx "github.com/DataDog/datadog-agent/comp/core/schema/fx-noop"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	extensiontypes "github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/types"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/input"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// defaultExtensionTimeout is the timeout of the ddflare extension request, matching the core Agent flare
	defaultExtensionTimeout = 20 * time.Second
	// debugSourceTimeout leaves room for the pprof CPU profile, which takes 30 seconds
	debugSourceTimeout = 45 * time.Second
	// noConfigProvided replaces the configurations when no --config is given. It isn't a YAML
	// comment, which the scrubber would remove.
	noConfigProvided = "No configuration files provided, pass the --config flags given to the collector"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*subcommands.GlobalParams

	// args are the positional command-line arguments
	args []string

	// subcommand-specific flags
	customerEmail string
	autoconfirm   bool
	keepArchive   bool
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

			flareParams := flare.NewLocalParams(
				"", // distPath - not used for OTel Agent
				"", // pyChecksPath - not used for OTel Agent
				"", // logFilePath - not used for OTel Agent
				"", // jmxLogFilePath - not used for OTel Agent
				"", // dogstatsDLogFilePath - not used for OTel Agent
				"", // streamlogsLogFilePath - not used for OTel Agent
			)
			flareParams.KeepArchiveAfterSend = cliParams.keepArchive
			return fxutil.OneShot(makeFlare,
				fx.Supply(cliParams),
				flareConfigModule(globalParams),
				fx.Supply(log.ForOneShot(globalParams.LoggerName, "info", false)),
				logfx.Module(),
				secretsnoopfx.Module(),
				delegatedauthnoopfx.Module(),
				flare.Module(flareParams),
				// Provide empty option for workloadmeta (optional dependency)
				fx.Supply(option.None[workloadmeta.Component]()),
				// Insecure so the flare is still created when the IPC artifacts are missing,
				// e.g. when the collector isn't running
				ipcfx.ModuleInsecure(),
			)
		},
	}

	flareCmd.Flags().StringVarP(&cliParams.customerEmail, "email", "e", "", "Your email")
	flareCmd.Flags().BoolVarP(&cliParams.autoconfirm, "send", "s", false, "Automatically send flare (don't prompt for confirmation)")
	flareCmd.Flags().BoolVar(&cliParams.keepArchive, "keep-archive", false, "Keep the flare archive file after a successful upload")
	flareCmd.SetArgs([]string{"caseID"})

	return flareCmd
}

// flareConfigModule provides the Datadog configuration used to reach the running collector and
// to send the flare. When collector configs are given, it is built from them like `otel-agent run`
// does, so the flare is uploaded with the datadog exporter's API key and site.
func flareConfigModule(params *subcommands.GlobalParams) fx.Option {
	if len(params.ConfPaths) > 0 {
		acfg, err := agentConfig.NewConfigComponent(context.Background(), params.CoreConfPath, params.ConfPaths)
		if err == nil || errors.Is(err, agentConfig.ErrNoDDExporter) {
			return fx.Provide(func() config.Component { return acfg })
		}
		fmt.Fprintln(color.Output, color.YellowString(fmt.Sprintf("Warning: could not build the configuration from the collector configuration: %v", err)))
	}
	return fx.Options(
		fx.Supply(config.NewAgentParams(params.CoreConfPath, config.WithConfigName(params.ConfigName))),
		config.Module(),
		schemanoopfx.Module(),
	)
}

func makeFlare(
	flareComp flare.Component,
	_ log.Component,
	cfg config.Component,
	ipcComp ipc.Component,
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
	fmt.Fprintln(color.Output, color.BlueString("Collecting diagnostic data from the OTel Agent..."))
	filePath, err := createOTelFlare(cliParams.GlobalParams, cfg, ipcComp.GetClient())
	if err != nil {
		return err
	}

	if cfg.GetString("api_key") == "" {
		fmt.Fprintln(color.Output, color.YellowString("Warning: no API key found in the collector configuration or DD_API_KEY, the upload will be rejected."))
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
	return e
}

// createOTelFlare collects diagnostic data and creates a flare archive
func createOTelFlare(params *subcommands.GlobalParams, cfg pkgconfigmodel.Reader, client ipc.HTTPClient) (string, error) {
	// Collect data
	data, err := collectOTelData(params, cfg, client)
	if err != nil {
		return "", fmt.Errorf("failed to collect OTel data: %w", err)
	}
	// Scrub the data before serializing it: once embedded in JSON, secrets are out of reach of the
	// text scrubber applied to every file of the archive.
	data, err = scrubResponse(data)
	if err != nil {
		return "", fmt.Errorf("failed to scrub OTel data: %w", err)
	}
	files := append(responseFiles(data), collectDebugSources(data.Sources)...)

	// Private, unpredictable dir (0700 on Unix; per-user temp ACL on Windows) so others can't read/symlink the flare.
	// Not RemoveAccessToOtherUsers: it grants dd-agent, not the caller, locking out a non-admin operator on Windows.
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir for flare: %w", err)
	}

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("otel-agent-flare_%s.zip", timestamp)
	filePath := filepath.Join(tmpDir, filename)

	if err := createFlareArchive(filePath, files); err != nil {
		return "", fmt.Errorf("failed to create flare archive: %w", err)
	}

	fmt.Fprintln(color.Output, color.GreenString("Flare archive created: "+filePath))
	return filePath, nil
}

// collectOTelData returns the flare data served by the ddflare extension of the running collector,
// like the core Agent flare does. When the collector can't be reached, it falls back to the data
// that can be read from the configuration files.
func collectOTelData(params *subcommands.GlobalParams, cfg pkgconfigmodel.Reader, client ipc.HTTPClient) (*extensiontypes.Response, error) {
	extensionURL := cfg.GetString("otelcollector.extension_url")
	fmt.Fprintf(color.Output, "Requesting the flare data of the running OTel Agent at %s...\n", extensionURL)
	resp, err := requestRunningCollector(cfg, client, extensionURL)
	if err == nil {
		return resp, nil
	}
	fmt.Fprintln(color.Output, color.YellowString(fmt.Sprintf("Warning: could not get the flare data of the running OTel Agent: %v", err)))
	fmt.Fprintln(color.Output, color.YellowString("Falling back to the configuration files: the flare won't contain the effective configuration."))
	return collectConfigData(params)
}

// requestRunningCollector requests the flare data from the ddflare extension of the running collector.
func requestRunningCollector(cfg pkgconfigmodel.Reader, client ipc.HTTPClient, extensionURL string) (*extensiontypes.Response, error) {
	timeout := time.Duration(cfg.GetInt("otelcollector.extension_timeout")) * time.Second
	if timeout <= 0 {
		timeout = defaultExtensionTimeout
	}
	body, err := client.Get(extensionURL, ipchttp.WithTimeout(timeout))
	if err != nil {
		return nil, err
	}
	var resp extensiontypes.Response
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("could not read the ddflare extension response: %w", err)
	}
	return &resp, nil
}

// collectDebugSources fetches the debug extension (pprof, zpages, health_check) endpoints, like the
// core Agent flare does for the embedded collector.
func collectDebugSources(sources map[string]extensiontypes.OTelFlareSource) []flareFile {
	client := &http.Client{Timeout: debugSourceTimeout}
	var files []flareFile
	for _, name := range slices.Sorted(maps.Keys(sources)) {
		for i, sourceURL := range sources[name].URLs {
			if !strings.HasPrefix(sourceURL, "http://") && !strings.HasPrefix(sourceURL, "https://") {
				sourceURL = "http://" + sourceURL
			}
			u, err := url.Parse(sourceURL)
			if err != nil {
				files = append(files, flareFile{name: fmt.Sprintf("otel-flare/%s_%d.err", name, i), content: []byte(err.Error()), scrub: true})
				continue
			}
			fileName := "otel-flare/" + name + strings.ReplaceAll(u.Path, "/", "_")
			fmt.Fprintf(color.Output, "Collecting %s...\n", sourceURL)
			content, scrub, err := fetchDebugSource(client, sourceURL)
			if err != nil {
				files = append(files, flareFile{name: fileName + ".err", content: []byte(err.Error()), scrub: true})
				continue
			}
			files = append(files, flareFile{name: fileName + ".dat", content: content, scrub: scrub})
		}
	}
	return files
}

// fetchDebugSource returns the content of a debug endpoint and whether it should be scrubbed:
// binary content (pprof profiles) is kept as is.
func fetchDebugSource(client *http.Client, sourceURL string) ([]byte, bool, error) {
	resp, err := client.Get(sourceURL)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("%s returned %s", sourceURL, resp.Status)
	}
	return content, resp.Header.Get("Content-Type") != "application/octet-stream", nil
}

// collectConfigData collects diagnostic data similar to ddflareextension from the configuration files
func collectConfigData(params *subcommands.GlobalParams) (*extensiontypes.Response, error) {
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
		customerConfig = noConfigProvided
		runtimeConfig = noConfigProvided
		envConfig = noConfigProvided
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
	// No remote providers. The env provider is required to resolve ${env:...} references,
	// otherwise the whole customer config is dropped.
	rs := confmap.ResolverSettings{
		URIs: uris,
		ProviderFactories: []confmap.ProviderFactory{
			fileprovider.NewFactory(),
			envprovider.NewFactory(),
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

// flareFile is a file of the flare archive
type flareFile struct {
	name    string
	content []byte
	// scrub is false for binary content only (pprof profiles)
	scrub bool
}

// scrubResponse returns a copy of the flare data with credentials scrubbed from the configurations
// and the environment, using the structured YAML and JSON scrubbers.
func scrubResponse(data *extensiontypes.Response) (*extensiontypes.Response, error) {
	scrubbed := *data
	for _, conf := range []*string{&scrubbed.CustomerConfig, &scrubbed.EnvConfig, &scrubbed.RuntimeOverrideConfig, &scrubbed.RuntimeConfig} {
		if *conf == "" || *conf == noConfigProvided {
			continue
		}
		s, err := scrubber.ScrubYamlString(*conf)
		if err != nil {
			return nil, err
		}
		*conf = s
	}

	if len(data.Environment) > 0 {
		envJSON, err := json.Marshal(data.Environment)
		if err != nil {
			return nil, err
		}
		if envJSON, err = scrubber.ScrubJSON(envJSON); err != nil {
			return nil, err
		}
		scrubbed.Environment = nil
		if err := json.Unmarshal(envJSON, &scrubbed.Environment); err != nil {
			return nil, err
		}
	}
	return &scrubbed, nil
}

// responseFiles returns the flare files built from the flare data
func responseFiles(data *extensiontypes.Response) []flareFile {
	var files []flareFile
	add := func(name string, content []byte) {
		files = append(files, flareFile{name: name, content: content, scrub: true})
	}
	addJSON := func(name string, v any) {
		content, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			add(name+".err", []byte(err.Error()))
			return
		}
		add(name, content)
	}

	addJSON("otel-response.json", data)
	add("build-info.txt", fmt.Appendf(nil, "Agent Version: %s\nAgent Command: %s\nAgent Description: %s\nExtension Version: %s\nBYOC: %v\n",
		data.AgentVersion, data.AgentCommand, data.AgentDesc, data.ExtensionVersion, data.BYOC))

	configs := []struct{ name, content string }{
		{"config/customer-config.yaml", data.CustomerConfig},
		{"config/env-config.yaml", data.EnvConfig},
		{"config/runtime-override-config.yaml", data.RuntimeOverrideConfig},
		{"config/runtime-config.yaml", data.RuntimeConfig},
	}
	for _, c := range configs {
		if c.content != "" {
			add(c.name, []byte(c.content))
		}
	}

	if len(data.Environment) > 0 {
		addJSON("environment.json", data.Environment)
	}
	if len(data.Sources) > 0 {
		addJSON("debug-sources.json", data.Sources)
	}
	return files
}

// createFlareArchive creates a zip archive with the given files, scrubbing their content
func createFlareArchive(filePath string, files []flareFile) error {
	// Parent dir is created private (0700) by createOTelFlare, so no other user can pre-seed a symlink here.
	zipFile, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, f := range files {
		content := f.content
		if f.scrub {
			// Fail closed: never write a file that could not be scrubbed
			if content, err = scrubber.ScrubBytes(content); err != nil {
				return fmt.Errorf("failed to scrub %s: %w", f.name, err)
			}
		}
		if err := addFileToZip(zipWriter, f.name, content); err != nil {
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
