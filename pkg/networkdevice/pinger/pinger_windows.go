//go:build windows

package pinger

// WindowsPinger is a structure for pinging
// hosts in Windows
type WindowsPinger struct {
	cfg Config
}

// New creates a WindowsPinger using the passed in
// config
func New(cfg Config) (Pinger, error) {
	if !cfg.UseRawSocket {
		return nil, ErrUDPSocketUnsupported
	}
	return &WindowsPinger{
		cfg: cfg,
	}, nil
}

// Ping takes a host sends ICMP ping
func (p *WindowsPinger) Ping(host string) (*Result, error) {
	// We set privileged to true, per pro-bing's docs
	// but it's not actually privileged
	// https://github.com/prometheus-community/pro-bing#windows
	return RunPing(&p.cfg, host)
}
