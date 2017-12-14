package custommetrics

import (
	"os"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/cmd/server"
	"github.com/spf13/pflag"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
)

var options *server.CustomMetricsAdapterServerOptions
var stopCh chan struct{}

func init() {
	options = server.NewCustomMetricsAdapterServerOptions(os.Stdout, os.Stdout) // FIXME: log to seelog
}

func AddFlags(fs *pflag.FlagSet) {
	options.SecureServing.AddFlags(fs)
	options.Authentication.AddFlags(fs)
	options.Authorization.AddFlags(fs)
	options.Features.AddFlags(fs)
}

func ValidateArgs(args []string) error {
	return options.Validate(args)
}

// StartServer creates and start a k8s custom metrics API server
func StartServer() error {
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
