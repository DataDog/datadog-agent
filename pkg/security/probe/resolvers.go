package probe

type Resolvers struct {
	DentryResolver *DentryResolver
	MountResolver *MountResolver
}

func (r *Resolvers) Start() error {
	return r.DentryResolver.Start()
}
