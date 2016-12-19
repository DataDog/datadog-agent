package app

import (
	"fmt"
	"net"
	"strings"

	log "github.com/cihub/seelog"
)

var (
	shouldStop chan bool
	listener   net.Listener
)

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
		case "stop":
			log.Debugf("Command `stop` received")
			shouldStop <- true
		default:
			log.Debugf("Unknown command: %v", data)
		}
	}
}

func cmdListen() {
	// setup a channel to handle stop requests
	shouldStop = make(chan bool)

	var err error
	listener, err = getListener()
	if err != nil {
		// we use the listener to stop the Agent, there's
		// no way we can handle this error
		panic(fmt.Sprintf("Unable to create the command server: %v", err))
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Errorf("Unable to accept command: %v", err)
			continue
		}
		go cmdHandler(conn)
	}
}

func cmdStopListen() {
	listener.Close()
}
