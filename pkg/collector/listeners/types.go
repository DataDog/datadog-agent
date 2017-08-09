package listeners

// ID is the representation of the unique ID of a Service
type ID string

// Service reprensents an application we can run a check against.
// It should be matched with a check template by the ConfigResolver using the
// ADIdentifiers field.
type Service struct {
	ID            ID                // unique ID
	ADIdentifiers []string          // identifiers on which templates will be matched
	Hosts         map[string]string // network --> IP address
	Ports         []int
	Tags          []string
}

// ServiceListener monitors running services and triggers check (un)scheduling
//
// It holds a cache of running services, listens to new/killed services and
// updates its cache, and the ConfigResolver with these events.
type ServiceListener interface {
	Listen()
	Stop()
}
