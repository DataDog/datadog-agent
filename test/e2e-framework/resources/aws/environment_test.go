// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aws

import (
	"context"
	"testing"

	awsECR "github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fakeRegistryID = "123456789012"
	fakeRegistry   = fakeRegistryID + ".dkr.ecr.us-east-1.amazonaws.com"
)

type fakeECR struct {
	out *awsECR.BatchGetImageOutput
	err error
	in  *awsECR.BatchGetImageInput
}

func (f *fakeECR) BatchGetImage(_ context.Context, in *awsECR.BatchGetImageInput, _ ...func(*awsECR.Options)) (*awsECR.BatchGetImageOutput, error) {
	f.in = in
	return f.out, f.err
}

func TestECRRepositoryName(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		expected string
	}{
		{
			name:     "pipeline image",
			image:    "669783387624.dkr.ecr.us-east-1.amazonaws.com/agent-qa",
			expected: "agent-qa",
		},
		{
			// Pull-through cache repositories are nested, so the repository name keeps
			// every segment after the host.
			name:     "pull-through cache image",
			image:    "669783387624.dkr.ecr.us-east-1.amazonaws.com/ecr-public/datadog/agent",
			expected: "ecr-public/datadog/agent",
		},
		{
			name:     "dockerhub mirror image",
			image:    "669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub/datadog/agent-dev",
			expected: "dockerhub/datadog/agent-dev",
		},
		{
			name:     "no host",
			image:    "agent",
			expected: "agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ecrRepositoryName(tt.image))
		})
	}
}

func TestECRImageTagExists(t *testing.T) {
	// Responses below are what ECR returns: a missing tag or a malformed tag is a successful call
	// with the problem listed in Failures, whereas a missing repository is an API error.
	notFound := types.ImageFailure{FailureCode: types.ImageFailureCodeImageNotFound, FailureReason: new("Requested image not found")}

	tests := []struct {
		name         string
		image        string
		out          *awsECR.BatchGetImageOutput
		err          error
		expectExists bool
		expectErr    string
	}{
		{
			name:         "image found",
			image:        fakeRegistry + "/agent-qa",
			out:          &awsECR.BatchGetImageOutput{Images: []types.Image{{}}},
			expectExists: true,
		},
		{
			name:         "image not found",
			image:        fakeRegistry + "/agent-qa",
			out:          &awsECR.BatchGetImageOutput{Failures: []types.ImageFailure{notFound}},
			expectExists: false,
		},
		{
			name:      "empty response",
			image:     fakeRegistry + "/agent-qa",
			out:       &awsECR.BatchGetImageOutput{},
			expectErr: "unable to get image 123456789012/agent-qa:1-abc-linux: got 0 images, expected 1",
		},
		{
			name:      "several images",
			image:     fakeRegistry + "/agent-qa",
			out:       &awsECR.BatchGetImageOutput{Images: []types.Image{{}, {}}},
			expectErr: "unable to get image 123456789012/agent-qa:1-abc-linux: got 2 images, expected 1",
		},
		{
			name:  "invalid tag",
			image: fakeRegistry + "/agent-qa",
			out: &awsECR.BatchGetImageOutput{Failures: []types.ImageFailure{{
				FailureCode:   types.ImageFailureCodeInvalidImageTag,
				FailureReason: new("Invalid request parameters: image tag should satisfy the regex '^[a-zA-Z0-9-_.]{1,300}$'"),
			}}},
			expectErr: "unable to get image 123456789012/agent-qa:1-abc-linux: InvalidImageTag: " +
				"Invalid request parameters: image tag should satisfy the regex '^[a-zA-Z0-9-_.]{1,300}$'",
		},
		{
			name:  "failure without reason",
			image: fakeRegistry + "/agent-qa",
			out: &awsECR.BatchGetImageOutput{Failures: []types.ImageFailure{{
				FailureCode: types.ImageFailureCodeUpstreamUnavailable,
			}}},
			expectErr: "unable to get image 123456789012/agent-qa:1-abc-linux: UpstreamUnavailable: ",
		},
		{
			name:  "several failures",
			image: fakeRegistry + "/agent-qa",
			out: &awsECR.BatchGetImageOutput{Failures: []types.ImageFailure{
				{FailureCode: types.ImageFailureCodeInvalidImageTag, FailureReason: new("bad tag")},
				{FailureCode: types.ImageFailureCodeUpstreamUnavailable},
			}},
			expectErr: "unable to get image 123456789012/agent-qa:1-abc-linux: InvalidImageTag: bad tag\n" +
				"unable to get image 123456789012/agent-qa:1-abc-linux: UpstreamUnavailable: ",
		},
		{
			// Only a single ImageNotFound is trusted to mean the tag is missing.
			name:  "image not found among several failures",
			image: fakeRegistry + "/agent-qa",
			out: &awsECR.BatchGetImageOutput{Failures: []types.ImageFailure{
				notFound,
				{FailureCode: types.ImageFailureCodeInvalidImageTag, FailureReason: new("bad tag")},
			}},
			expectErr: "unable to get image 123456789012/agent-qa:1-abc-linux: ImageNotFound: Requested image not found\n" +
				"unable to get image 123456789012/agent-qa:1-abc-linux: InvalidImageTag: bad tag",
		},
		{
			name:  "repository not found",
			image: fakeRegistry + "/agent-qa",
			err: &types.RepositoryNotFoundException{
				Message: new("The repository with name 'agent-qa' does not exist in the registry with id '123456789012'"),
			},
			expectErr: "RepositoryNotFoundException: The repository with name 'agent-qa' does not exist in the registry with id '123456789012'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeECR{out: tt.out, err: tt.err}

			exists, err := ecrImageTagExists(t.Context(), client, tt.image, "1-abc-linux")

			if tt.expectErr != "" {
				require.EqualError(t, err, tt.expectErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.expectExists, exists)
			assert.Equal(t, fakeRegistryID, *client.in.RegistryId)
			assert.Equal(t, ecrRepositoryName(tt.image), *client.in.RepositoryName)
			assert.Equal(t, "1-abc-linux", *client.in.ImageIds[0].ImageTag)
		})
	}
}

func TestInternalRegistryImageTagExistsNonECRImage(t *testing.T) {
	// An image outside ECR cannot be looked up, so it is assumed to exist without calling AWS.
	exists, err := (&Environment{}).InternalRegistryImageTagExists("registry.example.com/agent-qa", "1-abc-linux")

	require.NoError(t, err)
	assert.True(t, exists)
}
