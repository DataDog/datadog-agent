package ipc

import (
	"fmt"
	"net"
	"strings"

	log "github.com/cihub/seelog"
)

// string tokens we handle
const (
	STOP = "stop"
)

var (
	// ShouldStop is the channel used to communicate with
	// the rest of the components
	ShouldStop chan bool
	listener   net.Listener
)

// cmdHandler parses any string the server received, looking for
// valid commands.
// Valid commands are:
//  * stop
//
func cmdHandler(c net.Conn) {
	for {
		buf := make([]byte, 512)

		nr, err := c.Read(buf)
		if err != nil {
			log.Debugf("Unable to read from buffer: %v", err)
			return
		}

		data := strings.Trim(string(buf[0:nr]), " \n\r\t")
		switch data {
		case STOP:
			log.Debugf("Command `%v` received", STOP)
			ShouldStop <- true
		default:
			log.Debugf("Unknown command: %v", data)
		}
	}
}

// Listen listens for string to different type of connections, depending
// on the operating system. It processes any string it receives by invoking
// `cmdHandler`.
func Listen() {
	// setup a channel to handle stop requests
	ShouldStop = make(chan bool)

	go func() {
		var err error
		listener, err = getListener()
		if err != nil {
			// we use the listener to handle commands for the Agent, there's
			// no way we can handle this error
			panic(fmt.Sprintf("Unable to create the command server: %v", err))
		}

		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Errorf("Unable to accept commands: %v", err)
				continue
			}
			go cmdHandler(conn)
		}
	}()
}

// StopListen closes the connection and the server
// stops listening to new commands.
func StopListen() {
	listener.Close()
}
