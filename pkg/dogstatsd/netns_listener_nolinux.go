// +build !linux

package dogstatsd

func (s *Server) setupNetNsListeners() {
	// noop
}
