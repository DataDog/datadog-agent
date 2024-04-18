package collectorContrib

import (
	collectorContrib "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/def"
	//"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/fileexporter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/ackextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/asapauthextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/awsproxy"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/bearertokenauthextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/headerssetterextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/httpforwarderextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/jaegerremotesampling"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/oauth2clientauthextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/dockerobserver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/ecsobserver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/ecstaskobserver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/hostobserver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/k8sobserver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/oidcauthextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/opampextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/pprofextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/sigv4authextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/dbstorage"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/filestorage"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/debugexporter"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/ballastextension"
	"go.opentelemetry.io/collector/extension/zpagesextension"
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
		zpagesextension.NewFactory(),
		ballastextension.NewFactory(),
		ackextension.NewFactory(),
		asapauthextension.NewFactory(),
		awsproxy.NewFactory(),
		basicauthextension.NewFactory(),
		bearertokenauthextension.NewFactory(),
		headerssetterextension.NewFactory(),
		healthcheckextension.NewFactory(),
		httpforwarderextension.NewFactory(),
		jaegerremotesampling.NewFactory(),
		oauth2clientauthextension.NewFactory(),
		dockerobserver.NewFactory(),
		ecsobserver.NewFactory(),
		ecstaskobserver.NewFactory(),
		hostobserver.NewFactory(),
		k8sobserver.NewFactory(),
		oidcauthextension.NewFactory(),
		opampextension.NewFactory(),
		pprofextension.NewFactory(),
		sigv4authextension.NewFactory(),
		filestorage.NewFactory(),
		dbstorage.NewFactory(),
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
		//datadogexporter.NewFactory(),
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
