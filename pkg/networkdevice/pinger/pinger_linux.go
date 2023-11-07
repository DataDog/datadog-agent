//go:build linux

package pinger

import (
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	probing "github.com/prometheus-community/pro-bing"
)

type LinuxPinger struct{}

func NewPinger() Pinger {
	return &LinuxPinger{}
}

func (p *LinuxPinger) Ping(host string) (*probing.Statistics, error) {
	tu, err := net.GetRemoteSystemProbeUtil("/opt/datadog-agent/run/sysprobe.sock") // TODO: read the system probe config here, get the default going
	if err != nil {
		log.Warnf("could not initialize system-probe connection: %v (will only log every 10 minutes)", err)
		return nil, err
	}
	return tu.GetPing("my-client-id", host) // TODO: create a client ID and pass it here
}
