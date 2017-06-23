// +build gce

package hostname

import "github.com/DataDog/datadog-agent/pkg/util/gce"

func init() {
	RegisterHostnameProvider("gce", gce.HostnameProvider)
}
