package utils

import (
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/namer"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type RandomGenerator struct {
	e     config.Env
	namer namer.Namer
}

func NewRandomGenerator(e config.Env, name string, options ...func(*RandomGenerator)) *RandomGenerator {
	rand := &RandomGenerator{
		e:     e,
		namer: namer.NewNamer(e.Ctx(), "random-"+name),
	}
	for _, opt := range options {
		opt(rand)
	}

	return rand
}

func (r *RandomGenerator) RandomString(name string, length int, special bool) (*random.RandomString, error) {
	return random.NewRandomString(r.e.Ctx(), r.namer.ResourceName("random-string", name), &random.RandomStringArgs{
		Length:  pulumi.Int(length),
		Special: pulumi.Bool(special),
	}, r.e.WithProviders(config.ProviderRandom))
}
