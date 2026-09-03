// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
)

// recordingComponent captures the InstanceParams discovery builds, which is what the assertions
// below are really about: the parameters decide where the credential ends up.
type recordingComponent struct {
	*delegatedauthmock.Mock
	recorded []delegatedauth.InstanceParams
}

func newRecordingComponent() *recordingComponent {
	rec := &recordingComponent{}
	rec.Mock = &delegatedauthmock.Mock{
		AddInstanceFunc: func(_ context.Context, params delegatedauth.InstanceParams) error {
			rec.recorded = append(rec.recorded, params)
			return nil
		},
	}
	return rec
}

func discoverEndpoints(t *testing.T, additionalEndpoints map[string][]string) *recordingComponent {
	t.Helper()
	var yaml strings.Builder
	yaml.WriteString("additional_endpoints:\n")
	for domain, keys := range additionalEndpoints {
		fmt.Fprintf(&yaml, "  %q:\n", domain)
		for _, key := range keys {
			fmt.Fprintf(&yaml, "    - %q\n", key)
		}
	}

	rec := newRecordingComponent()
	configureAdditionalEndpointsDelegatedAuth(context.Background(), confFromYAML(t, yaml.String()), rec, nil)
	return rec
}

func TestMapDirectiveUsesIdentityCheckedWriteback(t *testing.T) {
	const domain = "https://app.datadoghq.com"
	rec := discoverEndpoints(t, map[string][]string{
		domain: {"DELA(org-uuid-1, aws)"},
	})

	require.Len(t, rec.recorded, 1)
	assert.Equal(t, "additional_endpoints", rec.recorded[0].AdditionalEndpointsConfigKey)
	assert.Equal(t, domain, rec.recorded[0].AdditionalEndpointDomain)
	assert.Equal(t, 0, rec.recorded[0].AdditionalEndpointKeyIndex)
	assert.Equal(t, domain, rec.recorded[0].TargetSite)
}

func TestDirectiveKeepsOriginalValueForWritebackTracking(t *testing.T) {
	rec := discoverEndpoints(t, map[string][]string{
		"https://app.datadoghq.com": {"DELA(org-uuid-1, aws)"},
	})

	require.Len(t, rec.recorded, 1)
	assert.Equal(t, "DELA(org-uuid-1, aws)", rec.recorded[0].AdditionalEndpointDirective)
}

// A directive registers an instance; a plain API key alongside it does not.
func TestOnlyDirectivesRegisterInstances(t *testing.T) {
	rec := discoverEndpoints(t, map[string][]string{
		"https://app.datadoghq.com":  {"a-real-api-key", "DELA(org-uuid-1, aws)"},
		"https://other.datadoghq.eu": {"another-real-key"},
	})

	require.Len(t, rec.recorded, 1)
	assert.Equal(t, "org-uuid-1", rec.recorded[0].OrgUUID)
}

func TestAPMAdditionalEndpointsAreOutsideTheInitialScope(t *testing.T) {
	rec := newRecordingComponent()
	config := confFromYAML(t, `
apm_config:
  additional_endpoints:
    "https://trace.agent.second-org.datadoghq.com":
      - "DELA(org-uuid-1, aws)"
`)

	configureAdditionalEndpointsDelegatedAuth(context.Background(), config, rec, nil)
	assert.Empty(t, rec.recorded)
}

// Two directives for the same org under the same domain are distinct destinations for the
// credential, so they must not collapse: the component keys instances by APIKeyConfigKey, and a
// collision would silently drop one of them.
func TestTwoDirectivesForOneOrgGetDistinctInstanceKeys(t *testing.T) {
	rec := discoverEndpoints(t, map[string][]string{
		"https://app.datadoghq.com": {
			"DELA(org-uuid-1, aws, region=us-east-1)",
			"DELA(org-uuid-1, aws, region=eu-west-1)",
		},
	})

	require.Len(t, rec.recorded, 2)
	assert.NotEqual(t, rec.recorded[0].APIKeyConfigKey, rec.recorded[1].APIKeyConfigKey)

	assert.Equal(t, 0, rec.recorded[0].AdditionalEndpointKeyIndex)
	assert.Equal(t, 1, rec.recorded[1].AdditionalEndpointKeyIndex)
}

// A directive naming a provider we cannot serve must register nothing. Registering it anyway would
// leave an endpoint buffering against a provider that can never resolve.
func TestUnsupportedProviderRegistersNothing(t *testing.T) {
	rec := discoverEndpoints(t, map[string][]string{
		"https://app.datadoghq.com": {"DELA(org-uuid-1, definitely-not-a-cloud)"},
	})

	assert.Empty(t, rec.recorded)
}

// A malformed directive must not register an instance either. It is still filtered out of the API
// keys by PartitionRealAndPendingKeys, so the endpoint stays inert rather than shipping the text.
func TestMalformedDirectiveRegistersNothing(t *testing.T) {
	for _, directive := range []string{
		"DELA()",
		"DELA(org-uuid-1)",
		"DELA(, aws)",
		"DELA(org-uuid-1, )",
		"DELA(org-uuid-1, aws, region)",
		"DELA(org-uuid-1, aws, =us-east-1)",
	} {
		t.Run(directive, func(t *testing.T) {
			rec := discoverEndpoints(t, map[string][]string{
				"https://app.datadoghq.com": {directive},
			})
			assert.Empty(t, rec.recorded)
		})
	}
}

func TestParseDelaDirective(t *testing.T) {
	directive, ok := parseDelaDirective("  DELA( org-uuid-1 , AWS , Region = us-east-1 )  ")
	require.True(t, ok)

	assert.Equal(t, "org-uuid-1", directive.orgUUID)
	assert.Equal(t, "aws", directive.provider, "provider must be matched case-insensitively")
	assert.Equal(t, "us-east-1", directive.params["region"], "param names must be matched case-insensitively")
}

// The directive's region must win over the agent-wide default, otherwise a per-endpoint override
// is silently ignored and the auth proof is exchanged in the wrong region.
func TestDirectiveRegionOverridesTheAgentDefault(t *testing.T) {
	directive, ok := parseDelaDirective("DELA(org-uuid-1, aws, region=eu-west-1)")
	require.True(t, ok)

	cfg, err := providerConfigForDirective(directive, &cloudauthconfig.AWSProviderConfig{Region: "us-east-1"})
	require.NoError(t, err)
	assert.Equal(t, "eu-west-1", cfg.(*cloudauthconfig.AWSProviderConfig).Region)
}

func TestDirectiveInheritsTheAgentRegionWhenItOmitsOne(t *testing.T) {
	directive, ok := parseDelaDirective("DELA(org-uuid-1, aws)")
	require.True(t, ok)

	cfg, err := providerConfigForDirective(directive, &cloudauthconfig.AWSProviderConfig{Region: "us-east-1"})
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", cfg.(*cloudauthconfig.AWSProviderConfig).Region)
}

// A directive with no region and no process-wide default must return nil so the component
// auto-detects the region from the environment. A non-nil config with an empty region would
// skip auto-detection and exchange the auth proof against the wrong (or no) region.
func TestDirectiveWithNoRegionAndNoDefaultReturnsNilForAutoDetection(t *testing.T) {
	directive, ok := parseDelaDirective("DELA(org-uuid-1, aws)")
	require.True(t, ok)

	cfg, err := providerConfigForDirective(directive, nil)
	require.NoError(t, err)
	assert.Nil(t, cfg, "no region from directive or default should yield nil for auto-detection")
}

// Every parameter spelling that parseDelaDirective accepts as a fallback must also be redacted for
// logging. These two used to be able to disagree - the parser lower-cased nothing and the redaction
// regex was case-insensitive - so a "Fallback=" spelling parsed as a real key and logged in clear.
func TestFallbackIsRedactedForEverySpellingThatParses(t *testing.T) {
	for _, directive := range []string{
		"DELA(org-uuid-1, aws, fallback=supersecret)",
		"DELA(org-uuid-1, aws, FALLBACK=supersecret)",
		"DELA(org-uuid-1, aws, Fallback = supersecret)",
	} {
		t.Run(directive, func(t *testing.T) {
			parsed, ok := parseDelaDirective(directive)
			require.True(t, ok)
			require.Equal(t, "supersecret", parsed.params["fallback"],
				"this spelling parses as a fallback, so redaction must cover it too")

			assert.NotContains(t, redactDelaDirectiveForLogging(directive), "supersecret")
		})
	}
}

func TestFallbackParameterIsIgnored(t *testing.T) {
	rec := discoverEndpoints(t, map[string][]string{
		"https://app.datadoghq.com": {"DELA(org-uuid-1, aws, fallback=static-key)"},
	})

	require.Len(t, rec.recorded, 1)
	assert.Empty(t, rec.recorded[0].FallbackAPIKey)
}

func discoverListShape(t *testing.T, entries []map[string]any) *recordingComponent {
	t.Helper()
	var yaml strings.Builder
	yaml.WriteString("logs_config:\n  force_use_http: true\n  additional_endpoints:\n")
	for _, e := range entries {
		first := true
		for _, k := range []string{"host", "api_key"} {
			v, ok := e[k]
			if !ok {
				continue
			}
			lead := "    - "
			if !first {
				lead = "      "
			}
			fmt.Fprintf(&yaml, "%s%s: %q\n", lead, k, v)
			first = false
		}
	}

	rec := newRecordingComponent()
	configureListShapeAdditionalEndpointsDelegatedAuth(context.Background(), confFromYAML(t, yaml.String()), rec, nil)
	return rec
}

func TestListShapeDirectiveUsesIdentityCheckedWriteback(t *testing.T) {
	rec := discoverListShape(t, []map[string]any{
		{"host": "org2.datadoghq.com", "api_key": "DELA(org-uuid-2, aws)"},
	})

	require.Len(t, rec.recorded, 1)
	assert.Equal(t, "logs_config.additional_endpoints", rec.recorded[0].AdditionalEndpointsListConfigKey)
	assert.Equal(t, 0, rec.recorded[0].ListEntryIndex)
	assert.Equal(t, "org2.datadoghq.com", rec.recorded[0].TargetSite)
	assert.Equal(t, "DELA(org-uuid-2, aws)", rec.recorded[0].AdditionalEndpointDirective)
	assert.NotEmpty(t, rec.recorded[0].AdditionalEndpointIdentity)
}

func TestListShapeDirectiveRequiresForcedHTTP(t *testing.T) {
	rec := newRecordingComponent()
	config := confFromYAML(t, `
logs_config:
  additional_endpoints:
    - host: org2.datadoghq.com
      api_key: DELA(org-uuid-2, aws)
`)

	configureListShapeAdditionalEndpointsDelegatedAuth(context.Background(), config, rec, nil)
	assert.Empty(t, rec.recorded)
}

func TestListShapeWritebackPreservesAPIKeyFieldCasing(t *testing.T) {
	rec := newRecordingComponent()
	config := confFromYAML(t, `
logs_config:
  force_use_http: true
  additional_endpoints:
    - host: org2.datadoghq.com
      API_KEY: DELA(org-uuid-2, aws)
`)

	configureListShapeAdditionalEndpointsDelegatedAuth(context.Background(), config, rec, nil)
	require.Len(t, rec.recorded, 1)
	assert.Equal(t, "logs_config.additional_endpoints", rec.recorded[0].AdditionalEndpointsListConfigKey)
}

// A plain api_key entry must not register an instance.
func TestListShapePlainKeyRegistersNothing(t *testing.T) {
	rec := discoverListShape(t, []map[string]any{
		{"host": "other.datadoghq.com", "api_key": "a-real-key"},
	})
	assert.Empty(t, rec.recorded)
}

// Without a host there is nothing to key the provider on, and the auth proof would be exchanged
// against the agent's own site rather than the entry's. Registering it anyway would produce a
// credential for the wrong org, so the entry is skipped instead.
func TestListShapeEntryWithoutAHostRegistersNothing(t *testing.T) {
	rec := discoverListShape(t, []map[string]any{
		{"api_key": "DELA(org-uuid-2, aws)"},
	})
	assert.Empty(t, rec.recorded)
}

// Two orgs shipping logs to the same host still need distinct instances and write-back paths.
func TestListShapeTwoOrgsOnOneHostStayDistinct(t *testing.T) {
	const host = "shared.datadoghq.com"
	rec := discoverListShape(t, []map[string]any{
		{"host": host, "api_key": "DELA(org-a, aws)"},
		{"host": host, "api_key": "DELA(org-b, aws)"},
	})

	require.Len(t, rec.recorded, 2)
	assert.NotEqual(t, rec.recorded[0].APIKeyConfigKey, rec.recorded[1].APIKeyConfigKey)
	assert.Equal(t, 0, rec.recorded[0].ListEntryIndex)
	assert.Equal(t, 1, rec.recorded[1].ListEntryIndex)
}

func TestConfigureDelegatedAuthReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := configureDelegatedAuth(ctx, confFromYAML(t, ""), newRecordingComponent())
	require.ErrorIs(t, err, context.Canceled)
}
