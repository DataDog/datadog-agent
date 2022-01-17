package json

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/netflow/goflow2/format/common"
	"github.com/golang/protobuf/proto"
	"github.com/netsampler/goflow2/format"
)

type JsonDriver struct {
}

func (d *JsonDriver) Prepare() error {
	common.HashFlag()
	common.SelectorFlag()
	return nil
}

func (d *JsonDriver) Init(context.Context) error {
	err := common.ManualHashInit()
	if err != nil {
		return err
	}
	return common.ManualSelectorInit()
}

func (d *JsonDriver) Format(data interface{}) ([]byte, []byte, error) {
	msg, ok := data.(proto.Message)
	if !ok {
		return nil, nil, fmt.Errorf("message is not protobuf")
	}

	key := common.HashProtoLocal(msg)
	return []byte(key), []byte(common.FormatMessageReflectJSON(msg, "")), nil
}

func init() {
	d := &JsonDriver{}
	format.RegisterFormatDriver("json", d)
}
