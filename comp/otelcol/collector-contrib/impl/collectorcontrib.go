package collectorContrib

import (
	collectorContrib "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/def"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/fileexporter"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/debugexporter"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/multierr"
)

type collectorContribImpl struct{}

func NewComponent() collectorContrib.Component {
	return &collectorContribImpl{}
}

// OTelComponentFactories returns all of the otel collector components that the collector-contrib supports
func (c *collectorContribImpl) OTelComponentFactories() (otelcol.Factories, error) {
	var errs error

	connectorsList := []connector.Factory{
		// TODO: all of the connectors from Core collector & collector contrib
	}
	connectors, err := connector.MakeFactoryMap(connectorsList...)
	if err != nil {
		errs = multierr.Append(errs, err)
	}

	extensionsList := []extension.Factory{
		// TODO: all of the extensions from Core collector & collector contrib
	}
	extensions, err := extension.MakeFactoryMap(extensionsList...)
	if err != nil {
		errs = multierr.Append(errs, err)
	}

	receiverList := []receiver.Factory{
		// TODO: all of the receivers from Core collector & collector contrib
	}
	receivers, err := receiver.MakeFactoryMap(receiverList...)
	if err != nil {
		errs = multierr.Append(errs, err)
	}

	processorList := []processor.Factory{
		// TODO: all of the processors from Core collector & collector contrib
	}
	processors, err := processor.MakeFactoryMap(processorList...)
	if err != nil {
		errs = multierr.Append(errs, err)
	}

	exporterList := []exporter.Factory{
		debugexporter.NewFactory(),
		fileexporter.NewFactory(),
		otlpexporter.NewFactory(),
		datadogexporter.NewFactory(),
	}
	exporters, err := exporter.MakeFactoryMap(exporterList...)
	if err != nil {
		errs = multierr.Append(errs, err)
	}

	factories := otelcol.Factories{
		Connectors: connectors,
		Extensions: extensions,
		Receivers:  receivers,
		Processors: processors,
		Exporters:  exporters,
	}
	return factories, errs
}
