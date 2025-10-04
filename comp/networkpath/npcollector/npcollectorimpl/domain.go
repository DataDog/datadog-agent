package npcollectorimpl

import (
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

const domainLookupExpiration = 5 * time.Minute

type domainResolver struct {
	lookupHostFn func(host string) (addrs []string, err error)
}

func newDomainResolver() *domainResolver {
	return &domainResolver{
		lookupHostFn: net.LookupHost,
	}
}

func (d *domainResolver) getIPToDomainMap(domains []string) (map[string]string, error) {
	ipToDomain := make(map[string]string)
	for _, domain := range domains {
		ips, err := cache.GetWithExpiration(domain, func() ([]string, error) {
			ips, err := d.lookupHostFn(domain)
			return ips, err
		}, domainLookupExpiration)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			ipToDomain[ip] = domain
		}
	}
	return ipToDomain, nil
}
