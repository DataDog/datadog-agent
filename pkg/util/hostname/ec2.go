// +build ec2

package hostname

import "github.com/DataDog/datadog-agent/pkg/util/ec2"

func init() {
	RegisterHostnameProvider("ec2", ec2.HostnameProvider)
}
