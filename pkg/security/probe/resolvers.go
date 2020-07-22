// +build linux_bpf

package probe

// Resolvers holds the list of the event attribute resolvers
type Resolvers struct {
	DentryResolver *DentryResolver
	MountResolver  *MountResolver
}

// Start the resolvers
func (r *Resolvers) Start() error {
	return r.DentryResolver.Start()
}
