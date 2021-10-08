// +build serverless

package util

import "context"

// HostnameData contains hostname and the hostname provider
// Copy of the original struct in hostname.go
type HostnameData struct {
	Hostname string
	Provider string
}

// HostnameProviderConfiguration is the key for the hostname provider associated to datadog.yaml
// Copy of the original struct in hostname.go
const HostnameProviderConfiguration = "configuration"

// Fqdn returns the FQDN for the host if any
func Fqdn(hostname string) string {
	return ""
}

func GetHostname(ctx context.Context) (string, error) {
	return "", nil
}

func GetHostnameData() (HostnameData, error) {
	return HostnameData{}, nil
}
