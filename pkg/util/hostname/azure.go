package hostname

import "github.com/DataDog/datadog-agent/pkg/util/cloudproviders/azure"

func init() {
	RegisterHostnameProvider("azure", azure.GetHostname)
}
