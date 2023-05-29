package defaultcomponents

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	exportersCount  = 12
	receiversCount  = 8
	extensionsCount = 7
	processorCount  = 12
)

func TestComponents(t *testing.T) {
	factories, err := Components()
	assert.NoError(t, err)
	exporters := factories.Exporters
	assert.Len(t, exporters, exportersCount)
	// aws exporters
	assert.NotNil(t, exporters["awsxray"])
	assert.NotNil(t, exporters["awsemf"])
	// core exporters
	assert.NotNil(t, exporters["logging"])
	assert.NotNil(t, exporters["otlp"])
	assert.NotNil(t, exporters["otlphttp"])
	// other exporters
	assert.NotNil(t, exporters["file"])
	assert.NotNil(t, exporters["datadog"])
	assert.NotNil(t, exporters["dynatrace"])
	assert.NotNil(t, exporters["sapm"])
	assert.NotNil(t, exporters["signalfx"])
	assert.NotNil(t, exporters["logzio"])
	assert.NotNil(t, exporters["kafka"])

	receivers := factories.Receivers
	assert.Len(t, receivers, receiversCount)
	// aws receivers
	assert.NotNil(t, receivers["awsecscontainermetrics"])
	assert.NotNil(t, receivers["awscontainerinsightreceiver"])
	assert.NotNil(t, receivers["awsxray"])
	assert.NotNil(t, receivers["statsd"])

	// core receivers
	assert.NotNil(t, receivers["otlp"])
	// other receivers
	assert.NotNil(t, receivers["zipkin"])
	assert.NotNil(t, receivers["jaeger"])
	assert.NotNil(t, receivers["kafka"])

	extensions := factories.Extensions
	assert.Len(t, extensions, extensionsCount)
	// aws extensions
	assert.NotNil(t, extensions["awsproxy"])
	assert.NotNil(t, extensions["ecs_observer"])
	assert.NotNil(t, extensions["sigv4auth"])
	// core extensions
	assert.NotNil(t, extensions["zpages"])
	assert.NotNil(t, extensions["memory_ballast"])
	// other extensions
	assert.NotNil(t, extensions["pprof"])
	assert.NotNil(t, extensions["health_check"])

	processors := factories.Processors
	assert.Len(t, processors, processorCount)
	// aws processors
	assert.NotNil(t, processors["experimental_metricsgeneration"])
	// core processors
	assert.NotNil(t, processors["batch"])
	assert.NotNil(t, processors["memory_limiter"])
	// other processors
	assert.NotNil(t, processors["attributes"])
	assert.NotNil(t, processors["resource"])
	assert.NotNil(t, processors["probabilistic_sampler"])
	assert.NotNil(t, processors["span"])
	assert.NotNil(t, processors["filter"])
	assert.NotNil(t, processors["metricstransform"])
	assert.NotNil(t, processors["resourcedetection"])
	assert.NotNil(t, processors["cumulativetodelta"])
	assert.NotNil(t, processors["deltatorate"])
}
