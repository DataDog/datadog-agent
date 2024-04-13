package checks

import (
	"encoding/json"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func pathForConn(conn *model.Connection, epforwarderComp eventplatform.Component) {
	var remoteAddr *model.Addr
	remoteAddr = conn.Raddr
	if remoteAddr.Ip == "127.0.0.1" {
		// skip local addr
		return
	}

	log.Warnf("Conn: %+v", conn)
	log.Warnf("remoteAddr: %+v", remoteAddr)

	cfg := traceroute.Config{
		DestHostname: remoteAddr.Ip,
		DestPort:     uint16(remoteAddr.Port),
		MaxTTL:       24,
		TimeoutMs:    1000,
	}

	tr := traceroute.New(cfg)
	path, err := tr.Run()
	if err != nil {
		log.Warnf("traceroute error: %+v", err)
	}
	log.Warnf("Network Path: %+v", path)

	epForwarder, ok := epforwarderComp.Get()
	if ok {
		payloadBytes, err := json.Marshal(path)
		if err != nil {
			log.Errorf("SendEventPlatformEventBlocking error: %s", err)
		} else {

			log.Warnf("Network Path MSG: %s", string(payloadBytes))
			m := message.NewMessage(payloadBytes, nil, "", 0)
			err = epForwarder.SendEventPlatformEventBlocking(m, eventplatform.EventTypeNetworkPath)
			if err != nil {
				log.Errorf("SendEventPlatformEventBlocking error: %s", err)
			}
		}
	}
}
