// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package sbom

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/DataDog/agent-payload/v5/sbom"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"gopkg.in/zorkian/go-datadog-api.v2"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"

	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

var GitCommit string

type k8sSuite struct {
	baseSuite[environments.Kubernetes]
	newProvisioner func(helmValues string) provisioners.Provisioner
	skipModes      []string
}

func (suite *k8sSuite) SetupSuite() {
	suite.baseSuite.SetupSuite()
	suite.clusterName = suite.Env().KubernetesCluster.ClusterName
}

func (suite *k8sSuite) TearDownSuite() {
	suite.baseSuite.TearDownSuite()

	color.NoColor = false
	c := color.New(color.Bold).SprintfFunc()
	suite.T().Log(c("The data produced and asserted by these tests can be viewed on this dashboard:"))
	c = color.New(color.Bold, color.FgBlue).SprintfFunc()
	suite.T().Log(c("https://dddev.datadoghq.com/dashboard/qcp-brm-ysc/e2e-tests-sbom-k8s?refresh_mode=paused&tpl_var_kube_cluster_name%%5B0%%5D=%s&tpl_var_fake_intake_task_family%%5B0%%5D=%s-fakeintake-ecs&from_ts=%d&to_ts=%d&live=false",
		suite.clusterName,
		suite.clusterName,
		suite.StartTime().UnixMilli(),
		suite.EndTime().UnixMilli(),
	))
}

// Once pulumi has finished to create a stack, it can still take some time for the images to be pulled,
// for the containers to be started, for the agent collectors to collect workload information
// and to feed workload meta and the tagger.
//
// We could increase the timeout of all tests to cope with the agent tagger warmup time.
// But in case of a single bug making a single tag missing from every metric,
// all the tests would time out and that would be a waste of time.
//
// It’s better to have the first test having a long timeout to wait for the agent to warmup,
// and to have the following tests with a smaller timeout.
//
// Inside a testify test suite, tests are executed in alphabetical order.
// The 00 in Test00UpAndRunning is here to guarantee that this test, waiting for the agent pods to be ready,
// is run first.
func (suite *k8sSuite) Test00UpAndRunning() {
	timeout := 10 * time.Minute
	// FIPS images are bigger and take longer to pull and start
	if suite.Env().Agent.FIPSEnabled {
		timeout = 20 * time.Minute
	}
	suite.testUpAndRunning(timeout)
}

// An agent restart (because of a health probe failure or because of a OOM kill for ex.)
// can cause a completely random failure on a completely random test.
// A metric can be fully missing if the agent is restarted when the metric is checked.
// Only a subset of tags can be missing if the agent has just restarted, but not all the
// collectors have finished to feed workload meta and the tagger.
// So, checking if any agent has restarted during the tests can be valuable for investigations.
//
// Inside a testify test suite, tests are executed in alphabetical order.
// The ZZ in TestZZUpAndRunning is here to guarantee that this test, is run last.
func (suite *k8sSuite) TestZZUpAndRunning() {
	suite.testUpAndRunning(1 * time.Minute)
}

func (suite *k8sSuite) testUpAndRunning(waitFor time.Duration) {
	ctx := context.Background()

	suite.Run("agent pods are ready and not restarting", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			linuxNodes, err := suite.Env().KubernetesCluster.Client().CoreV1().Nodes().List(ctx, metav1.ListOptions{
				LabelSelector: fields.AndSelectors(
					fields.OneTermEqualSelector("kubernetes.io/os", "linux"),
					fields.OneTermNotEqualSelector("eks.amazonaws.com/compute-type", "fargate"),
				).String(),
			})
			require.NoErrorf(c, err, "Failed to list Linux nodes")

			linuxPods, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("app", suite.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
			})
			require.NoErrorf(c, err, "Failed to list Linux datadog agent pods")

			assert.Len(c, linuxPods.Items, len(linuxNodes.Items))

			for _, pod := range linuxPods.Items {
				for _, containerStatus := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
					assert.Truef(c, containerStatus.Ready, "Container %s of pod %s isn't ready", containerStatus.Name, pod.Name)
					assert.Zerof(c, containerStatus.RestartCount, "Container %s of pod %s has restarted", containerStatus.Name, pod.Name)
				}
			}
		}, waitFor, 10*time.Second, "Not all agents eventually became ready in time.")
	})
}

// myCollectT does nothing more than "github.com/stretchr/testify/assert".CollectT
// It’s used only to get access to `errors` field which is otherwise private.
type myCollectT struct {
	*assert.CollectT

	errors []error
}

func (mc *myCollectT) Errorf(format string, args ...interface{}) {
	mc.errors = append(mc.errors, fmt.Errorf(format, args...))
	mc.CollectT.Errorf(format, args...)
}

type scanMethod struct {
	mode       string
	method     string
	helmValues string
}

type scanResult struct {
	app          string
	version      string
	expectedTags []*regexp.Regexp
	optionalTags []*regexp.Regexp
}

// ...existing code...
func (suite *k8sSuite) TestSBOM() {
	scanMethods := []scanMethod{
		{
			mode:       "default",
			method:     "filesystem",
			helmValues: ``,
		},
		/*
		   		{
		   			mode:   "tarball",
		   			method: "tarball",
		   			helmValues: `datadog:
		     sbom:
		       containerImage:
		         uncompressedLayersSupport: false
		   `,
		   		},
		*/
		{
			mode:   "overlayfs",
			method: "overlayfs",
			helmValues: `datadog:
  sbom:
    containerImage:
      uncompressedLayersSupport: true
      overlayFSDirectScan: true
`,
		},
	}

	images := []scanResult{
		{
			app:     "ghcr.io/datadog/apps-nginx-server",
			version: apps.Version,
			expectedTags: []*regexp.Regexp{
				regexp.MustCompile(`^git\.commit\.sha:[[:xdigit:]]{40}$`),
				regexp.MustCompile(`^git\.repository_url:https://github\.com/DataDog/test-infra-definitions$`),
			},
		},
		{
			app:     "ghcr.io/datadog/redis",
			version: apps.Version,
			expectedTags: []*regexp.Regexp{
				regexp.MustCompile(`^git\.commit\.sha:[[:xdigit:]]{40}$`),
				regexp.MustCompile(`^git\.repository_url:https://github\.com/DataDog/test-infra-definitions$`),
			},
		},
		{
			app:     "quay.io/coreos/etcd",
			version: "v3.5.1",
		},
	}

	for n, mode := range scanMethods {
		m := mode.mode
		method := mode.method
		helmValues := mode.helmValues

		for _, img := range images {
			appImage := img.app
			appShortImage := filepath.Base(appImage)
			appVersion := img.version

			suite.Run("sbom_mode="+m+",image="+appImage, func() {
				if slices.Contains(suite.skipModes, m) {
					suite.T().Skipf("Skipping scanning method '%s'", m)
					return
				}

				sendEvent := func(alertType, text string) {
					if _, err := suite.DatadogClient().PostEvent(&datadog.Event{
						Title: pointer.Ptr(suite.T().Name()),
						Text: pointer.Ptr(fmt.Sprintf(`%%%%%%
	`+"```"+`
	%s
	`+"```"+`
	%%%%%%`, text)),
						AlertType: &alertType,
						Tags: []string{
							"app:agent-new-e2e-tests-containers",
							"cluster_name:" + suite.clusterName,
							"sbom:" + appImage,
							"sbom_mode:" + m,
							"test:" + suite.T().Name(),
						},
					}); err != nil {
						suite.T().Logf("Failed to post event: %s", err)
					}
				}

				defer func() {
					if suite.T().Failed() {
						sendEvent("error", fmt.Sprintf("Failed finding the `%s` SBOM payload with proper tags", appImage))
					} else {
						sendEvent("success", "All good!")
					}
				}()

				if n > 0 {
					suite.Fakeintake.FlushServerAndResetAggregators()

					provisioner := suite.newProvisioner(helmValues)
					suite.UpdateEnv(provisioner)

					// Wait for agent pods to restart and stabilize before checking SBOMs
					suite.testUpAndRunning(1 * time.Minute)
				}

				suite.EventuallyWithTf(func(collect *assert.CollectT) {
					c := &myCollectT{
						CollectT: collect,
						errors:   []error{},
					}
					// To enforce the use of myCollectT instead
					collect = nil //nolint:ineffassign

					defer func() {
						if len(c.errors) == 0 {
							sendEvent("success", "All good!")
						} else {
							sendEvent("warning", errors.Join(c.errors...).Error())
						}
					}()

					sbomIDs, err := suite.Fakeintake.GetSBOMIDs()
					require.NoErrorf(c, err, "Failed to query fake intake")

					sbomIDs = lo.Filter(sbomIDs, func(id string, _ int) bool {
						return strings.HasPrefix(id, appImage)
					})

					require.NotEmptyf(c, sbomIDs, "No SBOM for %s yet", appImage)

					images := lo.FlatMap(sbomIDs, func(id string, _ int) []*aggregator.SBOMPayload {
						images, err := suite.Fakeintake.FilterSBOMs(id)
						assert.NoErrorf(c, err, "Failed to query fake intake")
						return images
					})

					require.NotEmptyf(c, images, "No SBOM payload yet")

					images = lo.Filter(images, func(image *aggregator.SBOMPayload, _ int) bool {
						return image.Status == sbom.SBOMStatus_SUCCESS
					})

					require.NotEmptyf(c, images, "No successful SBOM yet")

					images = lo.Filter(images, func(image *aggregator.SBOMPayload, _ int) bool {
						cyclonedx := image.GetCyclonedx()
						return cyclonedx != nil &&
							cyclonedx.Metadata != nil &&
							cyclonedx.Metadata.Component != nil
					})

					require.NotEmptyf(c, images, "No SBOM with complete CycloneDX")

					for _, image := range images {
						cyclonedx := image.GetCyclonedx()
						require.NotNil(c, cyclonedx.Metadata.Component.Properties)

						expectedTags := []*regexp.Regexp{
							regexp.MustCompile(`^architecture:(amd|arm)64$`),
							regexp.MustCompile(fmt.Sprintf(`^image_id:%s@sha256:`, regexp.QuoteMeta(appImage))),
							regexp.MustCompile(fmt.Sprintf(`^image_name:%s$`, regexp.QuoteMeta(appImage))),
							regexp.MustCompile(`^image_tag:` + regexp.QuoteMeta(appVersion) + `$`),
							regexp.MustCompile(`^os_name:linux$`),
							regexp.MustCompile(fmt.Sprintf(`^short_image:%s$`, appShortImage)),
							regexp.MustCompile(`^scan_method:` + method + `$`),
						}

						if len(img.expectedTags) != 0 {
							expectedTags = append(expectedTags, img.expectedTags...)
						}

						err = assertTags(image.GetTags(), expectedTags, img.optionalTags, false)
						assert.NoErrorf(c, err, "Tags mismatch")

						properties := lo.Associate(image.GetCyclonedx().Metadata.Component.Properties, func(property *cyclonedx_v1_4.Property) (string, string) {
							return property.Name, *property.Value
						})

						if assert.Contains(c, properties, "aquasecurity:trivy:RepoTag") {
							assert.Equal(c, appImage+":"+appVersion, properties["aquasecurity:trivy:RepoTag"])
						}

						if assert.Contains(c, properties, "aquasecurity:trivy:RepoDigest") {
							assert.Contains(c, properties["aquasecurity:trivy:RepoDigest"], appImage+"@sha256:")
						}

						assert.Greater(c, len(cyclonedx.Components), 1, "Less than 2 components in CycloneDX SBOM")
					}
				}, 4*time.Minute, 10*time.Second, "Failed finding the container image payload")
			})
		}
	}
}
