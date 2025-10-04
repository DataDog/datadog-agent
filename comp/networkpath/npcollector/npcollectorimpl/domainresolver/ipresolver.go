package domainresolver

type IpToDomainResolver struct {
	ipToDomainMap map[string]string
}

func NewIpToDomainResolver(ipToDomainMap map[string]string) *IpToDomainResolver {
	return &IpToDomainResolver{
		ipToDomainMap: ipToDomainMap,
	}
}
func (r *IpToDomainResolver) ResolveIPToDomain(ip string) string {
	return r.ipToDomainMap[ip]
}
