package domainresolver

import (
	"fmt"
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

const domainLookupExpiration = 5 * time.Minute

type DomainResolver struct {
	lookupHostFn func(host string) (addrs []string, err error)
}

func NewDomainResolver() *DomainResolver {
	return &DomainResolver{
		lookupHostFn: net.LookupHost,
	}
}

func (d *DomainResolver) getIPToDomainMap(domains []string) (map[string]string, []error) {
	var errList []error
	ipToDomain := make(map[string]string)
	for _, domain := range domains {
		ips, err := cache.GetWithExpiration(domain, func() ([]string, error) {
			ips, err := d.lookupHostFn(domain)
			return ips, err
		}, domainLookupExpiration)
		if err != nil {
			errList = append(errList, fmt.Errorf("error looking up IPs for domain %s: %s", domain, err))
			continue
		}
		for _, ip := range ips {
			ipToDomain[ip] = domain
		}
	}
	return ipToDomain, errList
}

func (d *DomainResolver) GetIPResolverForDomains(domains []string) (*IpToDomainResolver, []error) {
	domainMap, errors := d.getIPToDomainMap(domains)
	return NewIpToDomainResolver(domainMap), errors
}
