package logs

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/netflow/goflow2/format/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/golang/protobuf/proto"
	"strconv"
)

// Driver desc
type Driver struct {
	PacketsChannel chan string
}

// Prepare desc
func (d *Driver) Prepare() error {
	common.HashFlag()
	common.SelectorFlag()
	return nil
}

// Init desc
func (d *Driver) Init(context.Context) error {
	err := common.ManualHashInit()
	if err != nil {
		return err
	}
	return common.ManualSelectorInit()
}

// Format desc
func (d *Driver) Format(data interface{}) ([]byte, []byte, error) {


	msg, ok := data.(proto.Message)
	if !ok {
		return nil, nil, fmt.Errorf("message is not protobuf")
	}

	key := common.HashProtoLocal(msg)

	flow := common.FormatMessageReflectJSON(msg, "")
	d.PacketsChannel <- flow

	log.Debugf("flow: %v", flow)
	return []byte(key), []byte(common.FormatMessageReflectJSON(msg, "")), nil
}

func sanitizePort(port uint32) string {
	// TODO: this is a naive way to sanitze port
	var strPort string
	if port > 1024 {
		strPort = "redacted"
	} else {
		strPort = strconv.Itoa(int(port))
	}
	return strPort
}
