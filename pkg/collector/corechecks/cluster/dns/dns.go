package dns

import "net"

// Resolver abstracts away the system dependency on the net.LookupHost, allowing it to be mocked out in testing.
type Resolver func(name string) ([]string, error)

// StandardResolver is the default net.LookupHost DNS resolver
var StandardResolver = net.LookupHost

var _ Resolver = StandardResolver // Compile-time check
