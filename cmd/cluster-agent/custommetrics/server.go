package custommetrics

import (
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/cmd/server"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/dynamicmapper"
	"github.com/spf13/pflag"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
)

var options *server.CustomMetricsAdapterServerOptions
var stopCh chan struct{}

func init() {
	// FIXME: log to seelog
	options = server.NewCustomMetricsAdapterServerOptions(os.Stdout, os.Stdout)
}

// AddFlags ensures the required flags exist
func AddFlags(fs *pflag.FlagSet) {
	options.SecureServing.AddFlags(fs)
	options.Authentication.AddFlags(fs)
	options.Authorization.AddFlags(fs)
	options.Features.AddFlags(fs)
}

// ValidateArgs validates the custom metrics arguments passed
func ValidateArgs(args []string) error {
	return options.Validate(args)
}

// StartServer creates and start a k8s custom metrics API server
func StartServer() error {
	config, err := options.Config()
	if err != nil {
		return err
	}
	var clientConfig *rest.Config
	clientConfig, err = rest.InClusterConfig()

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(clientConfig)
	if err != nil {
		return log.Errorf("Unable to construct discovery client for dynamic client: %v", err)
	}

	dynamicMapper, err := dynamicmapper.NewRESTMapper(discoveryClient, apimeta.InterfacesForUnstructured, time.Second*5)
	if err != nil {
		return log.Errorf("Unable to construct dynamic discovery mapper: %v", err)
	}

	clientPool := dynamic.NewClientPool(clientConfig, dynamicMapper, dynamic.LegacyAPIPathResolverFunc)
	if err != nil {
		return log.Errorf("Unable to construct lister client to initialize provider: %v", err)
	}

	emProvider := custommetrics.NewDatadogProvider(clientPool, dynamicMapper)
	// As the Custom Metrics Provider is introduced, change the first emProvider to a cmProvider.
	server, err := config.Complete().New("datadog-custom-metrics-adapter", emProvider, emProvider)
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
