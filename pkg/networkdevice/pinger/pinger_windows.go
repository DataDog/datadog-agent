//go:build windows

package pinger

import probing "github.com/prometheus-community/pro-bing"

// WindowsPinger is a structure for pinging
// hosts in Windows
type WindowsPinger struct{}

func NewPinger() Pinger {
	return &WindowsPinger{}
}

func (p *WindowsPinger) Ping(host string) (*probing.Statistics, error) {
	// We set privileged to true, per pro-bing's docs
	// but it's not actually privileged
	// https://github.com/prometheus-community/pro-bing#windows
	return RunPing(host, true)
}
