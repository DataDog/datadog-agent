package common

type Package string
type Packages map[Package]string

const (
	DatadogAgentPackage          Package = "datadog-agent"
	DatadogInstallerPackage      Package = "datadog-installer"
	DatadogAPMInjectPackage      Package = "datadog-apm-inject"
	DatadogAPMLibraryJavaPackage Package = "datadog-apm-library-java"
)

func (p Packages) Install(pkg Package, version string) {
	p[pkg] = version
}
