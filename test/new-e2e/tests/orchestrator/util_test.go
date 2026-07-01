// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestrator

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// expectAtLeastOneResource is a helper to wait for a resource to appear in the orchestrator payloads
type expectAtLeastOneResource struct {
	filter  *fakeintake.PayloadFilter
	test    func(payload *aggregator.OrchestratorPayload) bool
	message string
	timeout time.Duration
}

// Assert waits for a resource to appear in the orchestrator payloads, then checks if any of the found payloads pass the
// supplied test function. If no matching resource is found within the timeout, the test fails.
func (e expectAtLeastOneResource) Assert(t *testing.T, client *fakeintake.Client) {
	giveup := time.Now().Add(e.timeout)
	var lastErr error
	fmt.Println("trying to " + e.message)
	for {
		payloads, err := getOrchestratorResources(client, e.filter)
		if err != nil {
			lastErr = err
		} else {
			for _, p := range payloads {
				if p != nil && e.test(p) {
					fmt.Println("success: " + e.message)
					return
				}
			}
		}
		//fmt.Printf("found %d resources\n", len(payloads))
		if time.Now().After(giveup) {
			break
		}
		time.Sleep(5 * time.Second)
	}
	if lastErr != nil {
		t.Errorf("failed to %s: last fakeintake error: %v", e.message, lastErr)
		return
	}
	t.Error("failed to " + e.message)
}

type manifest struct {
	Kind       string `yaml:"kind"`
	APIVersion string `yaml:"apiVersion"`
	Spec       struct {
		Group string `yaml:"group"`
		Names struct {
			Kind string `yaml:"kind"`
		} `yaml:"names"`
	} `yaml:"spec"`
	Metadata struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
}

// expectAtLeastOneManifest is a helper to wait for a manifest to appear in the orchestrator payloads
type expectAtLeastOneManifest struct {
	test    func(payload *aggregator.OrchestratorManifestPayload, manif manifest) bool
	message string
	timeout time.Duration
}

// Assert waits for a manifest to appear in the orchestrator payloads, then checks if any of the found payloads pass the
// supplied test function. If no matching manifest is found within the timeout, the test fails.
func (e expectAtLeastOneManifest) Assert(suite *k8sSuite) {
	giveup := time.Now().Add(e.timeout)
	var lastErr error
	fmt.Println("trying to " + e.message)
	for {
		payloads, err := getOrchestratorManifests(suite.Env().FakeIntake.Client())
		if err != nil {
			lastErr = err
		} else {
			//fmt.Printf("found %d manifests\n", len(payloads))
			for _, p := range payloads {
				manif := manifest{}
				err := yaml.Unmarshal(p.Manifest.Content, &manif)
				if err != nil {
					continue // unable to parse manifest content
				}
				//fmt.Printf("MANIF %d %d - %s %s - %s %s\n", p.Type, p.Manifest.Type, manif.APIVersion, manif.Kind, manif.Metadata.Name, manif.Metadata.Namespace)
				if p != nil && e.test(p, manif) {
					fmt.Println("success: " + e.message)
					return
				}
			}
		}
		if time.Now().After(giveup) {
			break
		}
		time.Sleep(5 * time.Second)
	}
	if lastErr != nil {
		suite.Failf("failed to "+e.message, "last fakeintake error: %v", lastErr)
		return
	}
	suite.Fail("failed to " + e.message)
}

func getOrchestratorResources(client *fakeintake.Client, filter *fakeintake.PayloadFilter) (payloads []*aggregator.OrchestratorPayload, err error) {
	defer func() {
		panicErr := toFakeintakeTimeoutError(recover())
		if panicErr != nil {
			err = panicErr
		}
	}()
	return client.GetOrchestratorResources(filter)
}

func getOrchestratorManifests(client *fakeintake.Client) (payloads []*aggregator.OrchestratorManifestPayload, err error) {
	defer func() {
		panicErr := toFakeintakeTimeoutError(recover())
		if panicErr != nil {
			err = panicErr
		}
	}()
	return client.GetOrchestratorManifests()
}

func toFakeintakeTimeoutError(p any) error {
	if p == nil {
		return nil
	}
	message := fmt.Sprint(p)
	if strings.HasPrefix(message, "fakeintake call timed out:") {
		return errors.New(message)
	}
	panic(p)
}
