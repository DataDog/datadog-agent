// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fakeintake

import (
	"strings"

	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
)

// NewPathPrefix creates the random path prefix a fakeintake accepts intake requests under, a UUID kept in the stack
// state so it doesn't change on every update.
//
// Fakeintake addresses get reused: an Agent left over from a previous stack can keep sending to the address of a new
// fakeintake. With a prefix per fakeintake, the new fakeintake rejects the payloads of the old Agent, which are sent
// under the prefix of the old fakeintake.
func NewPathPrefix(e config.Env, name string, opts ...pulumi.ResourceOption) (pulumi.StringOutput, error) {
	prefix, err := random.NewRandomUuid(e.Ctx(), e.CommonNamer().ResourceName(name, "path-prefix"), &random.RandomUuidArgs{},
		utils.MergeOptions(opts, e.WithProviders(config.ProviderRandom))...)
	if err != nil {
		return pulumi.StringOutput{}, err
	}
	return prefix.Result, nil
}

// NoPathPrefix is the path prefix of a fakeintake that accepts intake requests on any path.
func NoPathPrefix() pulumi.StringOutput {
	return pulumi.String("").ToStringOutput()
}

// PathPrefixArgs returns the fakeintake server arguments setting the given path prefix, none when it is empty.
func PathPrefixArgs(prefix string) []string {
	if prefix == "" {
		return nil
	}
	return []string{"-path-prefix=" + prefix}
}

// URLWithPathPrefix appends the path prefix, if any, to the base URL of a fakeintake.
func URLWithPathPrefix(baseURL, prefix string) string {
	if prefix == "" {
		return baseURL
	}
	return strings.TrimSuffix(baseURL, "/") + "/" + prefix
}

// PathPrefixPath returns the path prefix as the path_prefix of a logs endpoint: /<prefix>, or empty without prefix.
func PathPrefixPath(prefix string) string {
	if prefix == "" {
		return ""
	}
	return "/" + prefix
}
