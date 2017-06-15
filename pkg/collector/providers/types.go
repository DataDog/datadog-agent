package providers

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// ConfigProvider is the interface that wraps the Collect method
//
// Collect is responsible of populating a list of CheckConfig instances
// by retrieving configuration patterns from external resources: files
// on disk, databases, environment variables are just few examples.
//
// Any type implementing the interface will take care of any dependency
// or data needed to access the resource providing the configuration.
type ConfigProvider interface {
	Collect() ([]check.Config, error)
}

// Provider is the interface to access storage backend for autoconfig (etcd, zookeeper, ...)
type Provider interface {
	// List return the full path of every nodes inside a location
	List(key string) ([]string, error)
	// ListName return the name (not full Path) of every nodes inside a location
	ListName(key string) ([]string, error)
	// Get returns the value for a key
	Get(key string) ([]byte, error)
}

func init() {
	// Where to look for check templates if no custom path is defined
	config.Datadog.SetDefault("autoconf_template_dir", "/datadog/check_configs")
	config.Datadog.SetDefault("autoconf_connection_timeout", 1*time.Second)
}
