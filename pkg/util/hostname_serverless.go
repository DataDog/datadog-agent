// +build serverless

package util

// Fqdn returns the FQDN for the host if any
func Fqdn(hostname string) string {
	return ""
}

func GetHostname() (string, error) {
	return "", nil
}
