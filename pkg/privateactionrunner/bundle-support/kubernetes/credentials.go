package kubernetes

type KubeConfigCredential struct {
	Context string
}

// TODO - real implementation for parsing credentials
func parseAsKubeConfigCredentials(credential interface{}) (*KubeConfigCredential, bool) {
	return &KubeConfigCredential{
		Context: "docker-desktop", // Default context, can be overridden
	}, true
}

func parseAsServiceAccountCredentials(credential interface{}) bool {
	return false
}
