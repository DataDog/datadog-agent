//go:build darwin

package pinger

type MacPinger struct {
	cfg Config
}

func NewPinger(cfg Config) (Pinger, error) {
	if cfg.UseRawSocket {
		return nil, ErrRawSocketUnsupported
	}
	return &MacPinger{
		cfg: cfg,
	}, nil
}

func (p *MacPinger) Ping(host string) (*Result, error) {
	return RunPing(&p.cfg, host)
}
