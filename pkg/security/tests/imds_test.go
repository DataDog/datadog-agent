// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/tests/testutils"
)

func TestAWSIMDSv1Request(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
		// TODO: Oracle because we are missing offsets
		return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel() || kv.IsOpenSUSELeapKernel()
	})

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_aws_imds_v1_request",
			Expression: fmt.Sprintf(`imds.cloud_provider == "aws" && imds.aws.is_imds_v2 == false && imds.type == "request" && process.file.name == "%s"`, path.Base(executable)),
		},
	}

	// create dummy interface
	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	// create fake IMDS server
	imdsServerAddr := fmt.Sprintf("%s:%v", testutils.IMDSTestServerIP, testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err = testutils.StopIMDSserver(imdsServer); err != nil {
			t.Fatal(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{networkIngressEnabled: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("aws_imds_v1_request", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			response, err := http.Get(fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL))
			if err != nil {
				return fmt.Errorf("failed to query IMDS server: %v", err)
			}
			defer response.Body.Close()

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_aws_imds_v1_request")
			assert.Equal(t, "request", event.IMDS.Type, "wrong IMDS request type")
			assert.Equal(t, imdsServerAddr, event.IMDS.Host, "wrong IMDS request Host")
			assert.Equal(t, testutils.IMDSSecurityCredentialsURL, event.IMDS.URL, "wrong IMDS request URL")
			assert.Equal(t, "Go-http-client/1.1", event.IMDS.UserAgent, "wrong IMDS request user agent")

			test.validateIMDSSchema(t, event)
		})
	})
}

func TestAWSIMDSv1Response(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
		// TODO: Oracle because we are missing offsets
		return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel() || kv.IsOpenSUSELeapKernel()
	})

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		// this rule is added first on purpose to double-check that an IMDSv2 event doesn't trigger and IMDSv1 rule
		{
			ID:         "test_rule_aws_imds_v2_request",
			Expression: fmt.Sprintf(`imds.cloud_provider == "aws" && imds.aws.is_imds_v2 == true && imds.type == "request" && process.file.name == "%s"`, path.Base(executable)),
		},
		{
			ID:         "test_rule_aws_imds_v1_response",
			Expression: fmt.Sprintf(`imds.cloud_provider == "aws" && imds.aws.is_imds_v2 == false && imds.type == "response" && process.file.name == "%s"`, path.Base(executable)),
		},
	}

	// create dummy interface
	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	// create fake IMDS server
	imdsServerAddr := fmt.Sprintf("%s:%v", testutils.IMDSTestServerIP, testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err = testutils.StopIMDSserver(imdsServer); err != nil {
			t.Fatal(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{networkIngressEnabled: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("aws_imds_v1_response", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			response, err := http.Get(fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL))
			if err != nil {
				return fmt.Errorf("failed to query IMDS server: %v", err)
			}
			defer response.Body.Close()

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_aws_imds_v1_response")
			assert.Equal(t, "response", event.IMDS.Type, "wrong IMDS request Type")
			assert.Equal(t, testutils.AWSIMDSServerTestValue, event.IMDS.Server, "wrong IMDS request Server")
			assert.Equal(t, testutils.AWSSecurityCredentialsTypeTestValue, event.IMDS.AWS.SecurityCredentials.Type, "wrong IMDS request AWS Security Credentials Type")
			assert.Equal(t, testutils.AWSSecurityCredentialsExpirationTestValue, event.IMDS.AWS.SecurityCredentials.ExpirationRaw, "wrong IMDS request AWS Security Credentials ExpirationRaw")
			assert.Equal(t, testutils.AWSSecurityCredentialsAccessKeyIDTestValue, event.IMDS.AWS.SecurityCredentials.AccessKeyID, "wrong IMDS request AWS Security Credentials AccessKeyID")
			assert.Equal(t, testutils.AWSSecurityCredentialsCodeTestValue, event.IMDS.AWS.SecurityCredentials.Code, "wrong IMDS request AWS Security Credentials Code")
			assert.Equal(t, testutils.AWSSecurityCredentialsLastUpdatedTestValue, event.IMDS.AWS.SecurityCredentials.LastUpdated, "wrong IMDS request AWS Security Credentials LastUpdated")

			test.validateIMDSSchema(t, event)
		})
	})
}

func TestNoAWSIMDSv1Response(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
		// TODO: Oracle because we are missing offsets
		return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel() || kv.IsOpenSUSELeapKernel()
	})

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_aws_imds_v1_response",
			Expression: fmt.Sprintf(`imds.cloud_provider == "aws" && imds.aws.is_imds_v2 == false && imds.type == "response" && process.file.name == "%s"`, path.Base(executable)),
		},
	}

	// create dummy interface
	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	// create fake IMDS server
	imdsServerAddr := fmt.Sprintf("%s:%v", testutils.IMDSTestServerIP, testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err = testutils.StopIMDSserver(imdsServer); err != nil {
			t.Fatal(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{networkIngressEnabled: false}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("no_aws_imds_v1_response", func(t *testing.T) {
		if err := waitForIMDSResponseProbeEvent(test, func() error {
			response, err := http.Get(fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL))
			if err != nil {
				return fmt.Errorf("failed to query IMDS server: %v", err)
			}
			defer response.Body.Close()

			return nil
		}, path.Base(executable)); err == nil {
			t.Fatal("shouldn't get an event")
		}
	})
}

func TestAWSIMDSv2Request(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
		// TODO: Oracle because we are missing offsets
		return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel() || kv.IsOpenSUSELeapKernel()
	})

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		// this rule is added first on purpose to double-check that an IMDSv2 event doesn't trigger and IMDSv1 rule
		{
			ID:         "test_rule_aws_imds_v1_response",
			Expression: fmt.Sprintf(`imds.cloud_provider == "aws" && imds.aws.is_imds_v2 == false && imds.type == "response" && process.file.name == "%s"`, path.Base(executable)),
		},
		{
			ID:         "test_rule_aws_imds_v2_request",
			Expression: fmt.Sprintf(`imds.cloud_provider == "aws" && imds.aws.is_imds_v2 == true && imds.type == "request" && process.file.name == "%s"`, path.Base(executable)),
		},
	}

	// create dummy interface
	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	// create fake IMDS server
	imdsServerAddr := fmt.Sprintf("%s:%v", testutils.IMDSTestServerIP, testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err = testutils.StopIMDSserver(imdsServer); err != nil {
			t.Fatal(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{networkIngressEnabled: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("aws_imds_v2_request", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL), nil)
			if err != nil {
				return fmt.Errorf("failed to instantiate request: %v", err)
			}
			req.Header.Set("X-aws-ec2-metadata-token", "my_secret_token")
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to query IMDS server: %v", err)
			}
			defer response.Body.Close()

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_aws_imds_v2_request")
			assert.Equal(t, "request", event.IMDS.Type, "wrong IMDS request type")
			assert.Equal(t, imdsServerAddr, event.IMDS.Host, "wrong IMDS request Host")
			assert.Equal(t, testutils.IMDSSecurityCredentialsURL, event.IMDS.URL, "wrong IMDS request URL")
			assert.Equal(t, "Go-http-client/1.1", event.IMDS.UserAgent, "wrong IMDS request user agent")

			test.validateIMDSSchema(t, event)
		})
	})
}

func TestGCPIMDS(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
		// TODO: Oracle because we are missing offsets
		return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel() || kv.IsOpenSUSELeapKernel()
	})

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_gcp_imds_request",
			Expression: fmt.Sprintf(`imds.cloud_provider == "gcp" && imds.type == "request" && process.file.name == "%s"`, path.Base(executable)),
		},
	}

	// create dummy interface
	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	// create fake IMDS server
	imdsServerAddr := fmt.Sprintf("%s:%v", testutils.IMDSTestServerIP, testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err = testutils.StopIMDSserver(imdsServer); err != nil {
			t.Fatal(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{networkIngressEnabled: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("gcp_imds_request", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL), nil)
			if err != nil {
				return fmt.Errorf("failed to instantiate request: %v", err)
			}
			req.Header.Set("Metadata-Flavor", "Google")
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to query IMDS server: %v", err)
			}
			defer response.Body.Close()

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_gcp_imds_request")
			assert.Equal(t, "request", event.IMDS.Type, "wrong IMDS request type")
			assert.Equal(t, imdsServerAddr, event.IMDS.Host, "wrong IMDS request Host")
			assert.Equal(t, testutils.IMDSSecurityCredentialsURL, event.IMDS.URL, "wrong IMDS request URL")
			assert.Equal(t, "Go-http-client/1.1", event.IMDS.UserAgent, "wrong IMDS request user agent")

			test.validateIMDSSchema(t, event)
		})
	})
}

func TestAzureIMDS(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
		// TODO: Oracle because we are missing offsets
		return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel() || kv.IsOpenSUSELeapKernel()
	})

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_azure_imds_request",
			Expression: fmt.Sprintf(`imds.cloud_provider == "azure" && imds.type == "request" && process.file.name == "%s"`, path.Base(executable)),
		},
	}

	// create dummy interface
	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	// create fake IMDS server
	imdsServerAddr := fmt.Sprintf("%s:%v", testutils.IMDSTestServerIP, testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err = testutils.StopIMDSserver(imdsServer); err != nil {
			t.Fatal(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{networkIngressEnabled: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("azure_imds_request", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL), nil)
			if err != nil {
				return fmt.Errorf("failed to instantiate request: %v", err)
			}
			req.Header.Set("Metadata", "true")
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to query IMDS server: %v", err)
			}
			defer response.Body.Close()

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_azure_imds_request")
			assert.Equal(t, "request", event.IMDS.Type, "wrong IMDS request type")
			assert.Equal(t, imdsServerAddr, event.IMDS.Host, "wrong IMDS request Host")
			assert.Equal(t, testutils.IMDSSecurityCredentialsURL, event.IMDS.URL, "wrong IMDS request URL")
			assert.Equal(t, "Go-http-client/1.1", event.IMDS.UserAgent, "wrong IMDS request user agent")

			test.validateIMDSSchema(t, event)
		})
	})
}

func TestIBMIMDS(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
		// TODO: Oracle because we are missing offsets
		return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel() || kv.IsOpenSUSELeapKernel()
	})

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_idbm_imds_request",
			Expression: fmt.Sprintf(`imds.cloud_provider == "ibm" && imds.type == "request" && process.file.name == "%s"`, path.Base(executable)),
		},
	}

	// create dummy interface
	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	// create fake IMDS server
	imdsServerAddr := fmt.Sprintf("%s:%v", testutils.IMDSTestServerIP, testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err = testutils.StopIMDSserver(imdsServer); err != nil {
			t.Fatal(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{networkIngressEnabled: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("ibm_imds_request", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL), nil)
			if err != nil {
				return fmt.Errorf("failed to instantiate request: %v", err)
			}
			req.Header.Set("Metadata-Flavor", "ibm")
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to query IMDS server: %v", err)
			}
			defer response.Body.Close()

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_idbm_imds_request")
			assert.Equal(t, "request", event.IMDS.Type, "wrong IMDS request type")
			assert.Equal(t, imdsServerAddr, event.IMDS.Host, "wrong IMDS request Host")
			assert.Equal(t, testutils.IMDSSecurityCredentialsURL, event.IMDS.URL, "wrong IMDS request URL")
			assert.Equal(t, "Go-http-client/1.1", event.IMDS.UserAgent, "wrong IMDS request user agent")

			test.validateIMDSSchema(t, event)
		})
	})
}

func TestOracleIMDS(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
		// TODO: Oracle because we are missing offsets
		return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel() || kv.IsOpenSUSELeapKernel()
	})

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_oracle_imds_request",
			Expression: fmt.Sprintf(`imds.cloud_provider == "oracle" && imds.type == "request" && process.file.name == "%s"`, path.Base(executable)),
		},
	}

	// create dummy interface
	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	// create fake IMDS server
	imdsServerAddr := fmt.Sprintf("%s:%v", testutils.IMDSTestServerIP, testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err = testutils.StopIMDSserver(imdsServer); err != nil {
			t.Fatal(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{networkIngressEnabled: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("oracle_imds_request", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL), nil)
			if err != nil {
				return fmt.Errorf("failed to instantiate request: %v", err)
			}
			req.Header.Set("Authorization", "Bearer Oracle")
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to query IMDS server: %v", err)
			}
			defer response.Body.Close()

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_oracle_imds_request")
			assert.Equal(t, "request", event.IMDS.Type, "wrong IMDS request type")
			assert.Equal(t, imdsServerAddr, event.IMDS.Host, "wrong IMDS request Host")
			assert.Equal(t, testutils.IMDSSecurityCredentialsURL, event.IMDS.URL, "wrong IMDS request URL")
			assert.Equal(t, "Go-http-client/1.1", event.IMDS.UserAgent, "wrong IMDS request user agent")

			test.validateIMDSSchema(t, event)
		})
	})
}

func TestIMDSProcessContext(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
		// TODO: Oracle because we are missing offsets
		return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel() || kv.IsOpenSUSELeapKernel()
	})

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_oracle_imds_request",
			Expression: fmt.Sprintf(`imds.cloud_provider == "oracle" && imds.type == "request" && process.file.name == "%s"`, path.Base(executable)),
		},
		{
			ID:         "test_imds_process_context",
			Expression: `open.file.path == "{{.Root}}/test-open" && open.flags & O_CREAT != 0`,
		},
		// check dumps
	}

	// create dummy interface
	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	// create fake IMDS server
	imdsServerAddr := fmt.Sprintf("%s:%v", testutils.IMDSTestServerIP, testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err = testutils.StopIMDSserver(imdsServer); err != nil {
			t.Fatal(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{networkIngressEnabled: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, testFilePtr, err := test.Path("test-open")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("imds_process_context", ifSyscallSupported("SYS_OPEN", func(t *testing.T, syscallNB uintptr) {
		defer os.Remove(testFile)

		test.WaitSignal(t, func() error {
			// make request first to populate process cache
			response, err := http.Get(fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL))
			if err != nil {
				return fmt.Errorf("failed to query IMDS server: %v", err)
			}
			defer response.Body.Close()

			fd, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), syscall.O_CREAT, 0755)
			if errno != 0 {
				return error(errno)
			}
			return syscall.Close(int(fd))
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_imds_process_context")

			// check if the process has the correct IMDS credentials context
			assert.NotNil(t, event.ProcessCacheEntry.Process.AWSSecurityCredentials, "empty IMDS context")
			if len(event.ProcessCacheEntry.Process.AWSSecurityCredentials) > 0 {
				creds := event.ProcessCacheEntry.Process.AWSSecurityCredentials[0]
				assert.Equal(t, testutils.AWSSecurityCredentialsTypeTestValue, creds.Type, "wrong IMDS context AWS Security Credentials Type")
				assert.Equal(t, testutils.AWSSecurityCredentialsExpirationTestValue, creds.ExpirationRaw, "wrong IMDS context AWS Security Credentials ExpirationRaw")
				assert.Equal(t, testutils.AWSSecurityCredentialsAccessKeyIDTestValue, creds.AccessKeyID, "wrong IMDS context AWS Security Credentials AccessKeyID")
				assert.Equal(t, testutils.AWSSecurityCredentialsCodeTestValue, creds.Code, "wrong IMDS context AWS Security Credentials Code")
				assert.Equal(t, testutils.AWSSecurityCredentialsLastUpdatedTestValue, creds.LastUpdated, "wrong IMDS context AWS Security Credentials LastUpdated")
			}

			test.validateOpenSchema(t, event)
		})
	}))
}
