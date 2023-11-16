package secrets

// Params contains parameters for secrets, specifically whether the component is enabled
type Params struct {
	Enabled bool
}

// NewEnabledParams constructs params for an enabled component
func NewEnabledParams() Params {
	return Params{
		Enabled: true,
	}
}

// NewDisabledParams constructs params for a disabled component
func NewDisabledParams() Params {
	return Params{
		Enabled: false,
	}
}
