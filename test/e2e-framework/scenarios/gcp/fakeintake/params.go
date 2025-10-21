package fakeintake

import "github.com/DataDog/test-infra-definitions/common"

type Params struct {
	DDDevForwarding bool
	ImageURL        string
	Memory          int
	LoadBalancer    bool
	StoreStype      string
	RetentionPeriod string
}

type Option = func(*Params) error

// NewParams returns a new instance of Fakeintake Params
func NewParams(options ...Option) (*Params, error) {
	params := &Params{
		ImageURL:        "gcr.io/datadoghq/fakeintake:latest",
		DDDevForwarding: true,
		Memory:          1024,
		LoadBalancer:    false,
	}
	return common.ApplyOption(params, options)
}

// WithImageURL sets the URL of the image to use to define the fakeintake
func WithImageURL(imageURL string) Option {
	return func(p *Params) error {
		p.ImageURL = imageURL
		return nil
	}
}

// WithoutDDDevForwarding sets the flag to disable DD Dev forwarding
func WithoutDDDevForwarding() Option {
	return func(p *Params) error {
		p.DDDevForwarding = false
		return nil
	}
}

// WithMemory sets the amount (in MiB) of memory to allocate to the fakeintake
func WithMemory(memory int) Option {
	return func(p *Params) error {
		p.Memory = memory
		return nil
	}
}

// WithLoadBalancer enable load balancer in front of the fakeintake
func WithLoadBalancer() Option {
	return func(p *Params) error {
		p.LoadBalancer = true
		return nil
	}
}

// WithRetentionPeriod set the retention period for the fakeintake
func WithRetentionPeriod(retentionPeriod string) Option {
	return func(p *Params) error {
		p.RetentionPeriod = retentionPeriod
		return nil
	}
}

// WithStoreType set the store type for the fakeintake
func WithStoreType(storeType string) Option {
	return func(p *Params) error {
		p.StoreStype = storeType
		return nil
	}
}
