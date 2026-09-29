// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package safety

import "testing"

func TestEveryDestinationRequiresStaging(t *testing.T) {
	emptyEnv := func(string) string { return "" }
	for _, site := range []string{"", "datadoghq.com", "us3.datadoghq.com", "datad0g.com.evil.invalid"} {
		if _, err := (Config{Site: site}).Resolve(emptyEnv); err == nil {
			t.Errorf("accepted site %q", site)
		}
	}
	for _, dest := range destinations {
		for _, endpoint := range []string{"https://app.datadoghq.com", "https://custom.invalid", "https://datad0g.com.evil.invalid", "https://evildatad0g.com", "http://app.datad0g.com", "https://user:secret@app.datad0g.com", "https://app.datad0g.com?api_key=secret", "https://app.datad0g.com:8080", "https://app.datad0g.com/redirect", "https://.datad0g.com", "https://app.datad0g.com/#fragment"} {
			if _, err := (Config{Site: Site, Endpoints: map[Destination][]string{dest: {endpoint}}}).Resolve(emptyEnv); err == nil {
				t.Errorf("accepted %s endpoint %q", dest, endpoint)
			}
		}
	}
	resolved, err := (Config{Site: Site}).Resolve(emptyEnv)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != len(destinations) {
		t.Fatal("unresolved delivery route")
	}
	for _, endpoint := range []string{"https://app.datad0g.com", "https://softinv-intake.datad0g.com.:443/"} {
		if err := ValidateEndpoint(endpoint); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProductionEnvironmentCannotOverrideConfig(t *testing.T) {
	for _, key := range []string{"DD_SITE", "DD_DD_URL", "DD_PROCESS_CONFIG_ADDITIONAL_ENDPOINTS", "DD_NETWORK_DEVICES_METADATA_LOGS_DD_URL"} {
		_, err := (Config{Site: Site}).Resolve(func(k string) string {
			if k == key {
				return "production"
			}
			return ""
		})
		if err == nil {
			t.Errorf("accepted %s", key)
		}
	}
}
