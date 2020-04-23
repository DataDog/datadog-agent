package cloudfoundry

import (
	"net"
	"testing"

	"code.cloudfoundry.org/garden"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/stretchr/testify/assert"
)

func TestParseContainerPorts(t *testing.T) {
	info := garden.ContainerInfo{
		MappedPorts: []garden.PortMapping{
			{
				HostPort:      10,
				ContainerPort: 20,
			},
			{
				HostPort:      11,
				ContainerPort: 21,
			},
		},
		ExternalIP: "127.0.0.1",
	}
	addresses := parseContainerPorts(info)
	expected := []containers.NetworkAddress{
		{
			IP:       net.ParseIP("127.0.0.1"),
			Port:     10,
			Protocol: "tcp",
		},
		{
			IP:       net.ParseIP("127.0.0.1"),
			Port:     11,
			Protocol: "tcp",
		},
	}
	assert.Equal(t, expected, addresses)
}
