// +build serverless

package util

// Fqdn returns the FQDN for the host if any
func Fqdn(hostname string) string {
	return ""
}

func GetHostname() (string, error) {
	// TODO(remy): we should return the ARN here.
	return "", nil
}
