package common

import "github.com/DataDog/datadog-agent/pkg/fleet/installer"

const (
	DatadogAgentPackage            string = "datadog-agent"
	DatadogInstallerPackage        string = "datadog-installer"
	DatadogAPMInjectPackage        string = "datadog-apm-inject"
	DatadogAPMLibraryJavaPackage   string = "datadog-apm-library-java"
	DatadogAPMLibraryPythonPackage string = "datadog-apm-library-python"
	DatadogAPMLibraryRubyPackage   string = "datadog-apm-library-ruby"
	DatadogAPMLibraryJSPackage     string = "datadog-apm-library-js"
	DatadogAPMLibraryDotNetPackage string = "datadog-apm-library-dotnet"
	DatadogAPMLibraryPHPPackage    string = "datadog-apm-library-php"
)

type Packages struct {
	install          map[string]string
	versionOverrides map[string]string
	installOverrides map[string]string
}

func (p *Packages) Install(pkg string, version string) {
	p.install[pkg] = version
}

func (p *Packages) exec(installer installer.Installer) error {
	
}
