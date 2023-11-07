//go:build darwin

package pinger

import probing "github.com/prometheus-community/pro-bing"

type MacPinger struct{}

func NewPinger() Pinger {
	return &MacPinger{}
}

func (p *MacPinger) Ping(host string) (*probing.Statistics, error) {
	return RunPing(host, false)
}
