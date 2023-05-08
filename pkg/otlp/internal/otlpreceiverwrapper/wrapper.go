package otlpreceiverwrapper

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
)

const (
	maxRecvMsgSizeMib = 10
)

// This wrapper allows us to create a new default config for settings native to the OTLP receiver.
// These settings can also be manually configured in the "receivers" section of the datadog.yaml
type wrapperFactory struct {
	receiver.Factory
}

// CreateDefaultConfig overrides the default config function to add custom OTLP receiver settings.
func (f *wrapperFactory) CreateDefaultConfig() component.Config {
	cfg := f.Factory.CreateDefaultConfig()

	c := cfg.(*otlpreceiver.Config)
	c.Protocols.GRPC.MaxRecvMsgSizeMiB = maxRecvMsgSizeMib

	return c
}

func NewFactory() receiver.Factory {
	return &wrapperFactory{otlpreceiver.NewFactory()}
}
