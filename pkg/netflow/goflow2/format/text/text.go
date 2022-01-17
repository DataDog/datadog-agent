package text

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/netflow/goflow2/format/common"
	"github.com/golang/protobuf/proto"
	"github.com/netsampler/goflow2/format"
)

type TextDriver struct {
}

func (d *TextDriver) Prepare() error {
	common.HashFlag()
	common.SelectorFlag()
	return nil
}

func (d *TextDriver) Init(context.Context) error {
	err := common.ManualHashInit()
	if err != nil {
		return err
	}
	return common.ManualSelectorInit()
}

func (d *TextDriver) Format(data interface{}) ([]byte, []byte, error) {
	msg, ok := data.(proto.Message)
	if !ok {
		return nil, nil, fmt.Errorf("message is not protobuf")
	}

	key := common.HashProtoLocal(msg)
	return []byte(key), []byte(common.FormatMessageReflectText(msg, "")), nil
}

func init() {
	d := &TextDriver{}
	format.RegisterFormatDriver("text", d)
}
