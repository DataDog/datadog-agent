package protobuf

import (
	"context"
	"flag"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/netflow/goflow2/format/common"
	"github.com/golang/protobuf/proto"
	"github.com/netsampler/goflow2/format"
)

// Driver desc
type Driver struct {
	fixedLen bool
}

// Prepare desc
func (d *Driver) Prepare() error {
	common.HashFlag()
	flag.BoolVar(&d.fixedLen, "format.protobuf.fixedlen", false, "Prefix the protobuf with message length")
	return nil
}

// Init desc
func (d *Driver) Init(context.Context) error {
	return common.ManualHashInit()
}

// Format desc
func (d *Driver) Format(data interface{}) ([]byte, []byte, error) {
	msg, ok := data.(proto.Message)
	if !ok {
		return nil, nil, fmt.Errorf("message is not protobuf")
	}
	key := common.HashProtoLocal(msg)

	if !d.fixedLen {
		b, err := proto.Marshal(msg)
		return []byte(key), b, err
	}
	buf := proto.NewBuffer([]byte{})
	err := buf.EncodeMessage(msg)
	return []byte(key), buf.Bytes(), err
}

func init() {
	d := &Driver{}
	format.RegisterFormatDriver("pb", d)
}
