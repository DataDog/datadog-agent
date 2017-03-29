package api

import (
	"net"
	"net/http"

	"github.com/Microsoft/go-winio"
)

const pipename = `\\.\pipe\ddagent`

// getListener returns a listening connection to a Windows named
// pipe for IPC
func getListener() (net.Listener, error) {

	//SecurityDescriptor: "D:P(A;;FA;;;WD)", // "WD is EVERYONE"
	allowedGroups := []string{
		"(A;;FA;;;BA)", // BA is built-in administrators
		"(A;;FA;;;DA)", // DA is domain admins
		"(A;;FA;;;EA)", // EA is Enterprise admin
		"(A;;FA;;;LA)", // LA is Local admin
		"(A;;FA;;;SY)", // SY is local system

	}
	secDesc := "D:P"
	// domain admins and enterprise admins may not exist,
	// especially in non-domain machines.  Which causes ListenPipe
	// to fail.  So, check each one for existance before adding to
	// the final security descriptor.
	for _, desc := range allowedGroups {
		_, err := winio.SddlToSecurityDescriptor("D:P" + desc)
		if err == nil {
			secDesc += desc
		}
	}
	c := winio.PipeConfig{
		SecurityDescriptor: secDesc,
	}
	return winio.ListenPipe(pipename, &c)
}

// HTTP doesn't need anything from the transport, so we can use
// a named pipe
func fakeDial(proto, addr string) (conn net.Conn, err error) {
	return winio.DialPipe(pipename, nil)
}

// GetClient is a convenience function returning an http
// client suitable to use a named pipe transport
func GetClient() *http.Client {
	tr := &http.Transport{
		Dial: fakeDial,
	}
	return &http.Client{Transport: tr}
}
