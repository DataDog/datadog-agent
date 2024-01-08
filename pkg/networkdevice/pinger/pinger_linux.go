//go:build linux

package pinger

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type LinuxPinger struct {
	cfg Config
}

func NewPinger(cfg Config) (Pinger, error) {
	return &LinuxPinger{
		cfg: cfg,
	}, nil
}

func (p *LinuxPinger) Ping(host string) (*Result, error) {
	if !p.cfg.UseRawSocket {
		return RunPing(&p.cfg, host)
	}

	tu, err := net.GetRemoteSystemProbeUtil("/opt/datadog-agent/run/sysprobe.sock") // TODO: read the system probe config here, get the default going
	if err != nil {
		log.Warnf("could not initialize system-probe connection: %v (will only log every 10 minutes)", err)
		return nil, err
	}
	resp, err := tu.GetPing("my-client-id", host) // TODO: create a client ID and pass it here
	if err != nil {
		return nil, err
	}

	var result Result
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
