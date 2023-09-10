package listeners_interfaces

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// ContainerPort represents a network port in a Service.
type ContainerPort struct {
	Port int
	Name string
}

// Service represents an application we can run a check against.
// It should be matched with a check template by the ConfigResolver using the
// ADIdentifiers field.
type Service interface {
	GetServiceID() string                                // unique service name
	GetTaggerEntity() string                             // tagger entity name
	GetADIdentifiers(context.Context) ([]string, error)  // identifiers on which templates will be matched
	GetHosts(context.Context) (map[string]string, error) // network --> IP address
	GetPorts(context.Context) ([]ContainerPort, error)   // network ports
	GetTags() ([]string, error)                          // tags
	GetPid(context.Context) (int, error)                 // process identifier
	GetHostname(context.Context) (string, error)         // hostname.domainname for the entity
	IsReady(context.Context) bool                        // is the service ready
	GetCheckNames(context.Context) []string              // slice of check names defined in kubernetes annotations or container labels
	HasFilter(containers.FilterType) bool                // whether the service is excluded by metrics or logs exclusion config
	GetExtraConfig(string) (string, error)               // Extra configuration values

	// FilterTemplates filters the templates which will be resolved against
	// this service, in a map keyed by template digest.
	//
	// This method is called every time the configs for the service change,
	// with the full set of templates matching this service.  It must not rely
	// on any non-static information except the given configs, and it must not
	// modify the configs in the map.
	FilterTemplates(map[string]integration.Config)
}

// ServiceListener monitors running services and triggers check (un)scheduling
//
// It holds a cache of running services, listens to new/killed services and
// updates its cache, and the AutoConfig with these events.
type ServiceListener interface {
	Listen(newSvc, delSvc chan<- Service)
	Stop()
}

// Config represents autodiscovery listener config
type Config interface {
	IsProviderEnabled(string) bool
}
