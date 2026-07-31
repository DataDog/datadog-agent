// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !cloudauth_aws

package aws

import (
	"context"
	"errors"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/aws/creds"
)

// Supported reports whether this build compiled the AWS credential providers, and so whether
// Agent Cloud Auth can resolve credentials at all.
//
// Resolving credentials pulls in aws-sdk-go-v2/credentials and credentials/endpointcreds, which
// cost binary size. Flavors that cannot encounter AWS workload identity are built without the
// cloudauth_aws tag and get this file instead: the IoT Agent (embedded and edge devices) and the
// Heroku Agent (dynos on a PaaS). Neither can be issued an IRSA token, an ECS task role or an EKS
// Pod Identity, so the providers would never resolve anything there.
//
// Callers check this before enabling delegated auth, so resolveCredentials below is unreachable in
// practice. It exists to keep the package compiling and to fail loudly rather than silently if a
// future caller forgets the check.
const Supported = false

func (a *AWSAuth) resolveCredentials(_ context.Context, _ pkgconfigmodel.Reader) (*creds.SecurityCredentials, error) {
	return nil, errors.New("this Agent flavor was built without AWS Cloud Auth support (cloudauth_aws build tag)")
}
