package custommetrics

import (
	"os"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/cmd/server"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
)

var stopCh chan struct{}

// StartServer creates and start a k8s custom metrics API server
func StartServer() error {
	options := server.NewCustomMetricsAdapterServerOptions(os.Stdout, os.Stdout) // FIXME: log to seelog
	config, err := options.Config()
	if err != nil {
		return err
	}

	cmProvider := custommetrics.NewDatadogProvider()

	server, err := config.Complete().New("datadog-custom-metrics-adapter", cmProvider)
	if err != nil {
		return err
	}
	stopCh = make(chan struct{})
	return server.GenericAPIServer.PrepareRun().Run(stopCh)
}

// StopServer closes the connection and the server
// stops listening to new commands.
func StopServer() {
	if stopCh != nil {
		stopCh <- struct{}{}
	}
}
