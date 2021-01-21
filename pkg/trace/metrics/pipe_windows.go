package metrics

import (
	"time"

	"github.com/Microsoft/go-winio"
)

func dialPipe(path string, timeout *time.Duration) (*statsWriter, error) {
	c, err := winio.DialPipe(path, timeout)
	if err != nil {
		return nil, err
	}
	return &statsWriter{c}, nil
}
