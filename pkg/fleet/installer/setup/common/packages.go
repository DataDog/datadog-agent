package common

import "github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"

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

var (
	order = []string{
		DatadogInstallerPackage,
		DatadogAgentPackage,
		DatadogAPMInjectPackage,
		DatadogAPMLibraryJavaPackage,
		DatadogAPMLibraryPythonPackage,
		DatadogAPMLibraryRubyPackage,
		DatadogAPMLibraryJSPackage,
		DatadogAPMLibraryDotNetPackage,
		DatadogAPMLibraryPHPPackage,
	}
)

func (s *Setup) installPackages() error {
	for _, pkg := range order {
		if version, ok := s.Packages.install[pkg]; ok {
			url := oci.PackageURL(s.Env, pkg, version)
			err := s.installer.Install(s.Ctx, url, nil)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type Packages struct {
	install map[string]string
}

func (p *Packages) Install(pkg string, version string) {
	p.install[pkg] = version
}
