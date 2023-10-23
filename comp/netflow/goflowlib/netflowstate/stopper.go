package netflowstate

import (
	"errors"
)

// ErrAlreadyStarted error happens when you try to start twice a flow routine
var ErrAlreadyStarted = errors.New("the routine is already started")

// stopper mechanism, common for all the flow routines
type stopper struct {
	stopCh chan struct{}
}

func (s *stopper) start() error {
	if s.stopCh != nil {
		return ErrAlreadyStarted
	}
	s.stopCh = make(chan struct{})
	return nil
}

func (s *stopper) Shutdown() {
	if s.stopCh != nil {
		select {
		case <-s.stopCh:
		default:
			close(s.stopCh)
		}

		s.stopCh = nil
	}
}
