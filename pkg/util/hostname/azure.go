package hostname

import "github.com/DataDog/datadog-agent/pkg/util/azure"

func init() {
	RegisterHostnameProvider("azure", azure.GetHostname)
}
