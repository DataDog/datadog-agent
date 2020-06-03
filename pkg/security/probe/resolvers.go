package probe

type Resolvers struct {
	DentryResolver *DentryResolver
}

func (r *Resolvers) Start() error {
	return r.DentryResolver.Start()
}
