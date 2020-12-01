package dns

import "net"

type DNSResolver func(name string) ([]string, error)

var _ DNSResolver = net.LookupHost // Compile-time check
