//go:build darwin

package pinger

import probing "github.com/prometheus-community/pro-bing"

type MacPinger struct {
	cfg Config
}

func NewPinger(cfg Config) Pinger {
	return &MacPinger{
		cfg: cfg,
	}
}

func (p *MacPinger) Ping(host string) (*probing.Statistics, error) {
	return RunPing(&p.cfg, host, false)
}
