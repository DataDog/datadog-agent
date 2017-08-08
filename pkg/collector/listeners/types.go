package listeners

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// ID is the representation of the unique ID of a service
type ID string

// Service reprensents an application we can run a check against.
// It should be matched with a check template by the ConfigResolver.
type Service struct {
	ID       ID                // unique ID
	ConfigID check.ID          // key on which templates will be matched
	Hosts    map[string]string // network --> IP address
	Ports    []int
	Tags     []string
}

// ServiceListener monitors running services and triggers check (un)scheduling
//
// It holds a cache of running services, listens to new/killed services and
// updates its cache, and the ConfigResolver with these events.
type ServiceListener interface {
	Listen()
	Stop()
}
