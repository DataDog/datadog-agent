// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package utils

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
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
