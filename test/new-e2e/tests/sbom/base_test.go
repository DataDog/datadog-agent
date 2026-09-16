// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package sbom

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/samber/lo"
)

type baseSuite[Env any] struct {
	e2e.BaseSuite[Env]

	Fakeintake  *fakeintake.Client
	clusterName string
}

// sbomTargetsSuite carries the runtime-agnostic SBOM assertions (container image
// + host) shared by the kubeadm and standalone-Docker suites, so every runtime
// is verified against the exact same expected SBOM output. The concrete suites
// add their own SetupSuite and Test00UpAndRunning readiness gate.
type sbomTargetsSuite[Env any] struct {
	baseSuite[Env]

	// expectLayerDigests is set for runtimes that expose a real per-layer
	// manifest digest: containerd reads it from the content store and CRI-O from
	// the image manifest on disk. Docker exposes none, so its components must
	// carry an empty LayerDigest rather than a fabricated one.
	expectLayerDigests bool
}

func (suite *baseSuite[Env]) BeforeTest(suiteName, testName string) {
	suite.T().Logf("START  %s/%s %s", suiteName, testName, time.Now())
	suite.BaseSuite.BeforeTest(suiteName, testName)
}

func (suite *baseSuite[Env]) AfterTest(suiteName, testName string) {
	suite.T().Logf("FINISH %s/%s %s", suiteName, testName, time.Now())
	suite.BaseSuite.AfterTest(suiteName, testName)
}

func assertTags(actualTags []string, expectedTags []*regexp.Regexp, optionalTags []*regexp.Regexp, acceptUnexpectedTags bool) error {
	missingTags := make([]*regexp.Regexp, len(expectedTags))
	copy(missingTags, expectedTags)
	unexpectedTags := []string{}

	for _, actualTag := range actualTags {
		found := false
		for i, expectedTag := range missingTags {
			if expectedTag.MatchString(actualTag) {
				found = true
				missingTags[i] = missingTags[len(missingTags)-1]
				missingTags = missingTags[:len(missingTags)-1]
				break
			}
		}

		if !found {
			for _, optionalTag := range optionalTags {
				if optionalTag.MatchString(actualTag) {
					found = true
					break
				}
			}
		}

		if !found {
			unexpectedTags = append(unexpectedTags, actualTag)
		}
	}

	if (len(unexpectedTags) > 0 && !acceptUnexpectedTags) || len(missingTags) > 0 {
		errs := make([]error, 0, 2)
		if len(unexpectedTags) > 0 {
			errs = append(errs, fmt.Errorf("unexpected tags: %s", strings.Join(unexpectedTags, ", ")))
		}
		if len(missingTags) > 0 {
			errs = append(errs, fmt.Errorf("missing tags: %s", strings.Join(lo.Map(missingTags, func(re *regexp.Regexp, _ int) string { return re.String() }), ", ")))
		}
		return errors.Join(errs...)
	}

	return nil
}
