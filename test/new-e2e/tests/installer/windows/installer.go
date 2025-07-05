// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
)

// DatadogInstallerRunner represents an interface for the Datadog Installer
type DatadogInstallerRunner interface {
	// type  helpers
	SetBinaryPath(path string)

	// subcommands
	Version() (string, error)
	SetCatalog(newCatalog Catalog) (string, error)
	StartExperiment(packageName string, packageVersion string) (string, error)
	PromoteExperiment(packageName string) (string, error)
	StopExperiment(packageName string) (string, error)
	InstallPackage(packageName string, opts ...installer.PackageOption) (string, error)
	InstallExperiment(packageName string, opts ...installer.PackageOption) (string, error)
	RemovePackage(packageName string) (string, error)
	RemoveExperiment(packageName string) (string, error)
	Status() (string, error)
	Purge() (string, error)
	GarbageCollect() (string, error)
	SetConfigExperiment(config ConfigExperiment) (string, error)
	StartConfigExperiment(packageName string, config ConfigExperiment) (string, error)
	PromoteConfigExperiment(packageName string) (string, error)
	StopConfigExperiment(packageName string) (string, error)

	// MSI commands
	// TODO: we should separate installation from the command line interface
	Install(opts ...MsiOption) error
	Uninstall(opts ...MsiOption) error
}

// DatadogInstaller represents an interface to the Datadog Installer on the remote host.
type DatadogInstaller struct {
	binaryPath          string
	env                 *environments.WindowsHost
	defaultInstallerURL string
	outputDir           string
}

// NewDatadogInstaller instantiates a new instance of the Datadog Installer running
// on a remote Windows host.
func NewDatadogInstaller(env *environments.WindowsHost, msiURL, outputDir string) *DatadogInstaller {
	if outputDir == "" {
		outputDir = os.TempDir()
	}

	return &DatadogInstaller{
		binaryPath:          path.Join(consts.Path, consts.BinaryName),
		env:                 env,
		outputDir:           outputDir,
		defaultInstallerURL: msiURL,
	}
}

// SetBinaryPath sets the path to the Datadog Installer binary.
func (d *DatadogInstaller) SetBinaryPath(path string) {
	d.binaryPath = path
}

func (d *DatadogInstaller) execute(cmd string, options ...client.ExecuteOption) (string, error) {
	// Ensure the API key and site are set for telemetry
	apiKey := os.Getenv("DD_API_KEY")
	if apiKey == "" {
		var err error
		apiKey, err = runner.GetProfile().SecretStore().Get(parameters.APIKey)
		if apiKey == "" || err != nil {
			apiKey = "deadbeefdeadbeefdeadbeefdeadbeef"
		}
	}
	envVars := map[string]string{
		"DD_API_KEY": apiKey,
		"DD_SITE":    "datadoghq.com",
	}

	// Append the environment variables to any existing options
	options = append(options, client.WithEnvVariables(envVars))

	output, err := d.env.RemoteHost.Execute(fmt.Sprintf("& \"%s\" %s", d.binaryPath, cmd), options...)
	if err != nil {
		return output, err
	}
	return strings.TrimSpace(output), nil
}

// executeFromCopy executes a command using a copy of the Datadog Installer binary that is created
// outside of the install directory. This is useful for commands that may remove the installer binary
func (d *DatadogInstaller) executeFromCopy(cmd string, options ...client.ExecuteOption) (string, error) {
	// Create temp file
	tempFile, err := windowsCommon.GetTemporaryFile(d.env.RemoteHost)
	if err != nil {
		return "", err
	}
	defer d.env.RemoteHost.Remove(tempFile) //nolint:errcheck
	// ensure it has a .exe extension
	exeTempFile := tempFile + ".exe"
	defer d.env.RemoteHost.Remove(exeTempFile) //nolint:errcheck
	// must pass -Force b/c the temporary file is already created
	copyCmd := fmt.Sprintf(`Copy-Item -Force -Path "%s" -Destination "%s"`, d.binaryPath, exeTempFile)
	_, err = d.env.RemoteHost.Execute(copyCmd)
	if err != nil {
		return "", err
	}
	// Execute the command with the copied binary
	output, err := d.env.RemoteHost.Execute(fmt.Sprintf("& \"%s\" %s", exeTempFile, cmd), options...)
	if err != nil {
		return output, err
	}
	return strings.TrimSpace(output), nil
}

// Version returns the version of the Datadog Installer on the host.
func (d *DatadogInstaller) Version() (string, error) {
	return d.execute("version")
}

func (d *DatadogInstaller) runCommand(command, packageName string, opts ...installer.PackageOption) (string, error) {
	var packageConfigFound = false
	var packageConfig installer.TestPackageConfig
	for _, packageConfig = range installer.PackagesConfig {
		if packageConfig.Name == packageName {
			packageConfigFound = true
			break
		}
	}

	if !packageConfigFound {
		return "", fmt.Errorf("unknown package %s", packageName)
	}

	err := optional.ApplyOptions(&packageConfig, opts)
	if err != nil {
		return "", nil
	}

	registryTag := packageName
	if packageConfig.Alias != "" {
		registryTag = packageConfig.Alias
	}

	envVars := installer.InstallScriptEnvWithPackages(e2eos.AMD64Arch, []installer.TestPackageConfig{packageConfig})

	packageURL := fmt.Sprintf("oci://%s/%s:%s", packageConfig.Registry, registryTag, packageConfig.Version)

	return d.execute(fmt.Sprintf("%s %s", command, packageURL), client.WithEnvVariables(envVars))
}

// SetCatalog configures the catalog for the Datadog Installer daemon.
func (d *DatadogInstaller) SetCatalog(newCatalog Catalog) (string, error) {
	serializedCatalog, err := json.Marshal(newCatalog)
	if err != nil {
		return "", err
	}
	// s.T().Logf("Running: daemon set-catalog '%s'", string(serializedCatalog))
	// escaping the quotes really shouldn't be necessary because powershell will not parse them
	// when inside the single quotes but it seems like Golang is doing something weird with the
	// quotes, but only on Windows since this works fine on Linux without escaping.
	catalog := strings.ReplaceAll(string(serializedCatalog), `"`, `\"`)
	return d.execute(fmt.Sprintf("daemon set-catalog '%s'", catalog))
}

// ignoreEOF ignores EOF errors
//
// Prior to 7.68, the daemon kills the connection for daemon commands that restart the daemon.
// Starting with 7.68, the daemon responds properly so the tests assert for it
func ignoreEOF(err error) error {
	if err != nil && strings.Contains(err.Error(), "EOF") {
		return nil
	}
	return err
}

// StartExperiment will use the Datadog Installer service to start an experiment.
func (d *DatadogInstaller) StartExperiment(packageName string, packageVersion string) (string, error) {
	return d.execute(fmt.Sprintf("daemon start-experiment '%s' '%s'", packageName, packageVersion))
}

// PromoteExperiment will use the Datadog Installer service to promote an experiment.
func (d *DatadogInstaller) PromoteExperiment(packageName string) (string, error) {
	return d.execute(fmt.Sprintf("daemon promote-experiment '%s'", packageName))
}

// StopExperiment will use the Datadog Installer service to stop an experiment.
func (d *DatadogInstaller) StopExperiment(packageName string) (string, error) {
	return d.execute(fmt.Sprintf("daemon stop-experiment '%s'", packageName))
}

// InstallPackage will attempt to use the Datadog Installer to install the package given in parameter.
//
// By default, it will use the package artifact from the current pipeline.
//
// Note: This command is a direct call to the installer and does not use the daemon.
func (d *DatadogInstaller) InstallPackage(packageName string, opts ...installer.PackageOption) (string, error) {
	return d.runCommand("install", packageName, opts...)
}

// InstallExperiment will attempt to use the Datadog Installer to start an experiment for the package given in parameter.
func (d *DatadogInstaller) InstallExperiment(packageName string, opts ...installer.PackageOption) (string, error) {
	return d.runCommand("install-experiment", packageName, opts...)
}

// RemovePackage requests that the Datadog Installer removes a package on the remote host.
func (d *DatadogInstaller) RemovePackage(packageName string) (string, error) {
	return d.execute(fmt.Sprintf("remove %s", packageName))
}

// RemoveExperiment requests that the Datadog Installer removes a package on the remote host.
func (d *DatadogInstaller) RemoveExperiment(packageName string) (string, error) {
	return d.execute(fmt.Sprintf("remove-experiment %s", packageName))
}

// Status returns the status provided by the running daemon
func (d *DatadogInstaller) Status() (string, error) {
	return d.execute("status")
}

// Purge runs the purge command, removing all packages.
func (d *DatadogInstaller) Purge() (string, error) {
	// executeFromCopy is used here because the installer will remove itself
	// if purge is run from the install directory it may cause an uninstall failure due
	// to the file being in use.
	return d.executeFromCopy("purge")
}

// GarbageCollect runs the garbage-collect command, removing unused packages.
func (d *DatadogInstaller) GarbageCollect() (string, error) {
	return d.execute("garbage-collect")
}

// Install will attempt to install the Datadog Agent on the remote host.
// By default, it will use the installer from the current pipeline.
func (d *DatadogInstaller) Install(opts ...MsiOption) error {
	params := MsiParams{
		msiLogFilename: "install.log",
		Params: Params{
			installerURL: d.defaultInstallerURL,
		},
	}
	err := optional.ApplyOptions(&params, opts)
	if err != nil {
		return err
	}

	// MSI can install from a URL or a local file
	remoteMSIPath := params.installerURL
	if strings.HasPrefix(remoteMSIPath, "file://") {
		// developer provided a local file, put it on the remote host
		localMSIPath := strings.TrimPrefix(remoteMSIPath, "file://")
		remoteMSIPath, err = windowsCommon.GetTemporaryFile(d.env.RemoteHost)
		if err != nil {
			return err
		}
		d.env.RemoteHost.CopyFile(localMSIPath, remoteMSIPath)
	}
	if remoteMSIPath == "" {
		return fmt.Errorf("MSI URL/path is required but was not provided")
	}
	logPath := filepath.Join(d.outputDir, params.msiLogFilename)
	if _, err := os.Stat(logPath); err == nil {
		return fmt.Errorf("log file %s already exists", logPath)
	}
	msiArgList := params.msiArgs[:]
	if params.agentUser != "" {
		msiArgList = append(msiArgList, fmt.Sprintf("DDAGENTUSER_NAME=%s", params.agentUser))
	}
	msiArgs := ""
	if msiArgList != nil {
		msiArgs = strings.Join(msiArgList, " ")
	}
	return windowsCommon.InstallMSI(d.env.RemoteHost, remoteMSIPath, msiArgs, logPath)
}

// Uninstall will attempt to uninstall the Datadog Agent on the remote host.
func (d *DatadogInstaller) Uninstall(opts ...MsiOption) error {
	params := MsiParams{
		msiLogFilename: "uninstall.log",
	}
	err := optional.ApplyOptions(&params, opts)
	if err != nil {
		return err
	}

	productCode, err := windowsCommon.GetProductCodeByName(d.env.RemoteHost, "Datadog Agent")
	if err != nil {
		return err
	}

	logPath := filepath.Join(d.outputDir, params.msiLogFilename)
	if _, err := os.Stat(logPath); err == nil {
		return fmt.Errorf("log file %s already exists", logPath)
	}
	msiArgs := ""
	if params.msiArgs != nil {
		msiArgs = strings.Join(params.msiArgs, " ")
	}
	return windowsCommon.MsiExec(d.env.RemoteHost, "/x", productCode, msiArgs, logPath)
}

// createFileRegistryFromLocalOCI uploads a local OCI package to the remote host and prepares it to
// be used as a `file://` package path for the daemon downloader.
//
// returns the path to the extracted package on the remote host.
//
// Currently, this requires extracting the OCI package to a directory.
func createFileRegistryFromLocalOCI(host *components.RemoteHost, localPackagePath string) (string, error) {
	// Upload OCI package to temporary path
	remotePath, err := windowsCommon.GetTemporaryFile(host)
	if err != nil {
		return "", err
	}
	host.CopyFile(localPackagePath, remotePath)
	// Extract OCI package
	outPath := remotePath + ".extracted"
	// tar is a built-in command on Windows 10+
	cmd := fmt.Sprintf("mkdir %s; tar -xf %s -C %s", outPath, remotePath, outPath)
	_, err = host.Execute(cmd)
	if err != nil {
		return "", err
	}
	// return path to extracted package
	return outPath, nil
}

// CreatePackageSourceIfLocal will create a package on the remote host if the URL is a local file.
// This is useful for development to test local packages.
func CreatePackageSourceIfLocal(host *components.RemoteHost, pkg TestPackageConfig) (TestPackageConfig, error) {
	url := pkg.URL()
	// If the URL is a file, upload it to the remote host
	if strings.HasPrefix(url, "file://") {
		localPath := strings.TrimPrefix(url, "file://")
		outPath, err := createFileRegistryFromLocalOCI(host, localPath)
		if err != nil {
			return pkg, err
		}
		// Must replace slashes so that daemon can parse it correctly
		outPath = strings.ReplaceAll(outPath, "\\", "/")
		pkg.urloverride = fmt.Sprintf("file://%s", outPath)
	}
	return pkg, nil
}

// NewPackageConfig is a struct that regroups the fields necessary to install a package from an OCI Registry
func NewPackageConfig(opts ...PackageOption) (TestPackageConfig, error) {
	c := TestPackageConfig{}
	for _, opt := range opts {
		err := opt(&c)
		if err != nil {
			return c, err
		}
	}
	if c.Alias == "" {
		switch c.Name {
		case consts.AgentPackage:
			c.Alias = "agent-package"
		}
	}
	for _, opt := range opts {
		err := opt(&c)
		if err != nil {
			return c, err
		}
	}
	return c, nil
}

// TestPackageConfig is a struct that regroups the fields necessary to install a package from an OCI Registry
type TestPackageConfig struct {
	// Name the name of the package
	Name string
	// Alias Sometimes the package is named differently in some registries
	Alias string
	// Version the version to install
	Version string
	// Registry the URL of the registry
	Registry string
	// Auth the authentication method, "" for no authentication
	Auth string
	// urloverride to use for package
	//
	// The URL is normally constructed from the above parts, this field will take precedence.
	// Useful for development to test local packages.
	urloverride string
}

// URL returns the OCI URL of the package
//
// It may begin with `file://` if the package is local.
func (c TestPackageConfig) URL() string {
	if c.urloverride != "" {
		// if the URL had been overridden, use it
		return c.urloverride
	}
	// else construct it from parts
	name := c.Name
	if c.Alias != "" {
		name = c.Alias
	}
	return fmt.Sprintf("oci://%s/%s:%s", c.Registry, name, c.Version)
}

// PackageOption is an optional function parameter type for the Datadog Installer
type PackageOption func(*TestPackageConfig) error

// WithName uses a specific name for the package.
func WithName(name string) PackageOption {
	return func(params *TestPackageConfig) error {
		params.Name = name
		return nil
	}
}

// WithAuthentication uses a specific authentication for a Registry to install the package.
func WithAuthentication(auth string) PackageOption {
	return func(params *TestPackageConfig) error {
		params.Auth = auth
		return nil
	}
}

// WithRegistry uses a specific Registry from where to install the package.
func WithRegistry(registryURL string) PackageOption {
	return func(params *TestPackageConfig) error {
		params.Registry = registryURL
		return nil
	}
}

// WithVersion uses a specific version of the package.
func WithVersion(version string) PackageOption {
	return func(params *TestPackageConfig) error {
		params.Version = version
		return nil
	}
}

// WithAlias specifies the package's alias.
func WithAlias(alias string) PackageOption {
	return func(params *TestPackageConfig) error {
		params.Alias = alias
		return nil
	}
}

// WithURLOverride specifies the package's URL.
func WithURLOverride(url string) PackageOption {
	return func(params *TestPackageConfig) error {
		params.urloverride = url
		return nil
	}
}

// WithPipeline configures the package to be installed from a pipeline.
func WithPipeline(pipeline string) PackageOption {
	return func(params *TestPackageConfig) error {
		params.Version = fmt.Sprintf("pipeline-%s", pipeline)
		if err := WithRegistry(consts.PipelineOCIRegistry)(params); err != nil {
			return err
		}
		return nil
	}
}

// WithPackage copies fields from the package
func WithPackage(pkg TestPackageConfig) PackageOption {
	return func(params *TestPackageConfig) error {
		*params = pkg
		return nil
	}
}

// WithDevEnvOverrides applies overrides to the package config based on environment variables.
//
// Example: local OCI package file
//
//	export CURRENT_AGENT_OCI_URL="file:///path/to/oci/package.tar"
//
// Example: from a different pipeline
//
//	export CURRENT_AGENT_OCI_PIPELINE="123456"
//
// Example: from a different pipeline
// (assumes that the package being overridden is already from a pipeline)
//
//	export CURRENT_AGENT_OCI_VERSION="pipeline-123456"
//
// Example: custom URL
//
//	export CURRENT_AGENT_OCI_URL="oci://installtesting.datad0g.com/agent-package:pipeline-123456"
func WithDevEnvOverrides(prefix string) PackageOption {
	return func(params *TestPackageConfig) error {
		// env vars for convenience
		if url, ok := os.LookupEnv(fmt.Sprintf("%s_OCI_URL", prefix)); ok {
			err := WithURLOverride(url)(params)
			if err != nil {
				return err
			}
		}
		if pipeline, ok := os.LookupEnv(fmt.Sprintf("%s_OCI_PIPELINE", prefix)); ok {
			err := WithPipeline(pipeline)(params)
			if err != nil {
				return err
			}
		}

		// env vars for specific fields
		if version, ok := os.LookupEnv(fmt.Sprintf("%s_OCI_VERSION", prefix)); ok {
			err := WithVersion(version)(params)
			if err != nil {
				return err
			}
		}
		if registry, ok := os.LookupEnv(fmt.Sprintf("%s_OCI_REGISTRY", prefix)); ok {
			err := WithRegistry(registry)(params)
			if err != nil {
				return err
			}
		}
		if auth, ok := os.LookupEnv(fmt.Sprintf("%s_OCI_AUTH", prefix)); ok {
			err := WithAuthentication(auth)(params)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

// SetConfigExperiment sets the config catalog for the Datadog Installer daemon.
func (d *DatadogInstaller) SetConfigExperiment(config ConfigExperiment) (string, error) {
	serializedConfig, err := json.Marshal(map[string]ConfigExperiment{
		config.ID: config,
	})
	if err != nil {
		return "", err
	}
	// Escape quotes in the JSON string to handle PowerShell quoting properly
	configStr := strings.ReplaceAll(string(serializedConfig), `"`, `\"`)
	return d.execute(fmt.Sprintf("daemon set-config-catalog '%s'", configStr))
}

// StartConfigExperiment starts a config experiment using the provided InstallerConfig through the daemon.
// It first sets the config catalog and then starts the experiment.
func (d *DatadogInstaller) StartConfigExperiment(packageName string, config ConfigExperiment) (string, error) {
	// First set the config catalog
	_, err := d.SetConfigExperiment(config)
	if err != nil {
		return "", err
	}
	// Then start the config experiment
	return d.execute(fmt.Sprintf("daemon start-config-experiment %s %s", packageName, config.ID))
}

// PromoteConfigExperiment promotes a config experiment through the daemon.
func (d *DatadogInstaller) PromoteConfigExperiment(packageName string) (string, error) {
	return d.execute(fmt.Sprintf("daemon promote-config-experiment %s", packageName))
}

// StopConfigExperiment stops a config experiment through the daemon.
func (d *DatadogInstaller) StopConfigExperiment(packageName string) (string, error) {
	return d.execute(fmt.Sprintf("daemon stop-config-experiment %s", packageName))
}

// ConfigExperiment represents a configuration experiment for the Datadog Installer.
type ConfigExperiment struct {
	ID    string                 `json:"id"`
	Files []ConfigExperimentFile `json:"files"`
}

// ConfigExperimentFile represents a configuration file in a config experiment.
type ConfigExperimentFile struct {
	Path     string          `json:"path"`
	Contents json.RawMessage `json:"contents"`
}

// DatadogInstallerGA represents an interface to the Datadog Installer on the remote host for GA versions (7.65.x).
// It handles special cases for the 7.65.x versions of the installer.
//
// We still check the version because the version may change during the test run, e.g. during MustStartExperiment,
// so this type mainly serves to keep the special case logic out of the normal DatadogInstaller type so we don't
// unintentially apply it to other tests.
type DatadogInstallerGA struct {
	*DatadogInstaller
}

// StartExperiment will use the Datadog Installer service to start an experiment.
// For 7.65.x versions, it uses the start-installer-experiment subcommand.
func (d *DatadogInstallerGA) StartExperiment(packageName string, packageVersion string) (string, error) {
	if packageName == consts.AgentPackage {
		ver, err := d.Version()
		if err != nil {
			return "", err
		}
		if strings.HasPrefix(ver, "7.65.") {
			return d.StartInstallerExperiment(consts.AgentPackage, packageVersion)
		}
		out, err := d.DatadogInstaller.StartExperiment(packageName, packageVersion)
		return out, ignoreEOF(err)
	}
	return d.DatadogInstaller.StartExperiment(packageName, packageVersion)
}

// StartInstallerExperiment will use the Datadog Installer service to start an experiment for the Agent package.
//
// Only needed for 7.65, future versions use the start-experiment subcommand instead.
func (d *DatadogInstallerGA) StartInstallerExperiment(packageName string, packageVersion string) (string, error) {
	out, err := d.execute(fmt.Sprintf("daemon start-installer-experiment '%s' '%s'", packageName, packageVersion))
	return out, ignoreEOF(err)
}

// StopExperiment will use the Datadog Installer service to stop an experiment for the Agent package.
//
// Workarounds:
// - ignore EOF errors
func (d *DatadogInstallerGA) StopExperiment(packageName string) (string, error) {
	out, err := d.DatadogInstaller.StopExperiment(packageName)
	if packageName == consts.AgentPackage {
		return out, ignoreEOF(err)
	}
	return out, err
}
