// +build docker

package hostname

import "github.com/DataDog/datadog-agent/pkg/util/docker"

func init() {
	RegisterHostnameProvider("docker", docker.HostnameProvider)
}
