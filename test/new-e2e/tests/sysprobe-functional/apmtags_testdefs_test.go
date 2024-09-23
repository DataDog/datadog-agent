// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sysprobefunctional

var usmTaggingTests = []usmTaggingTest{
	{
		name:            "all values json test 1",
		description:     "Basic test with all values from json",
		clientJSONFile:  "usmtest/client_all.json",
		clientAppConfig: "",
		defaultFiles: usmTaggingFiles{
			jsonFile:      "usmtest/defaultsite_all.json",
			appConfigFile: "",
		},
		serverSiteName: "", // empty is default site
		serverSitePort: "80",
		expectedClientTags: []string{
			"service:webclient_json",
			"env:testing_env_json",
			"version:1_json",
		},
		expectedServerTags: []string{
			"service:defaultsite_json",
			"env:testing_env_json",
			"version:1_json",
			"http.iis.site:1",
			"http.iis.app_pool:DefaultAppPool",
			"'http.iis.sitename:Default Web Site'",
		},
	},
	{
		name:            "all values xml test 1",
		description:     "Test with both json and app config provided, xml supercedes json",
		clientJSONFile:  "usmtest/client_all.json",
		clientAppConfig: "usmtest/client_all.xml",
		defaultFiles: usmTaggingFiles{
			jsonFile:      "usmtest/defaultsite_all.json",
			appConfigFile: "usmtest/defaultsite_all.xml",
		},
		serverSiteName: "", // empty is default site
		serverSitePort: "80",
		expectedClientTags: []string{
			"service:webclient_xml",
			"env:testing_env_xml",
			"version:1_xml",
		},
		expectedServerTags: []string{
			"service:defaultsite_xml",
			"env:testing_env_xml",
			"version:1_xml",
			"http.iis.site:1",
			"http.iis.app_pool:DefaultAppPool",
			"'http.iis.sitename:Default Web Site'",
		},
	},
	{
		name:            "test different site",
		description:     "Test different site in same IIS server, tests correct path discovery",
		clientJSONFile:  "usmtest/client_all.json",
		clientAppConfig: "",
		defaultFiles: usmTaggingFiles{
			jsonFile:      "usmtest/defaultsite_all.json",
			appConfigFile: "usmtest/defaultsite_all.xml",
		},
		siteFiles: map[string]usmTaggingFiles{
			"TestSite1": {
				jsonFile: "usmtest/site1.json",
			},
		},
		serverSiteName: "TestSite1", // empty is default site
		serverSitePort: "8081",
		expectedClientTags: []string{
			"service:webclient_json",
			"env:testing_env_json",
			"version:1_json",
		},
		expectedServerTags: []string{
			"service:site1_json",
			"env:testing_env_json",
			"version:1_json",
			"http.iis.site:2",
			"http.iis.app_pool:DefaultAppPool",
			"'http.iis.sitename:TestSite1'",
		},
	},
	{
		name:            "test site with application",
		description:     "Test different site in same IIS server, tests correct path discovery with an application",
		clientJSONFile:  "usmtest/client_all.json",
		clientAppConfig: "",
		defaultFiles: usmTaggingFiles{
			jsonFile:      "usmtest/defaultsite_all.json",
			appConfigFile: "usmtest/defaultsite_all.xml",
		},
		siteFiles: map[string]usmTaggingFiles{
			"TestSite1": {
				jsonFile: "usmtest/site1.json",
			},
		},
		appFiles: map[string]usmTaggingFiles{
			"/site1/app1": {
				jsonFile: "usmtest/site1_app1.json",
			},
			"/site1/app2": {
				jsonFile: "usmtest/site1_app2.json",
			},
			"/site1/app2/nested": {
				jsonFile: "usmtest/app2_nested.json",
			},
		},
		serverSiteName: "TestSite1", // empty is default site
		serverSitePort: "8081",
		targetPath:     "/site1/app1",
		expectedClientTags: []string{
			"service:webclient_json",
			"env:testing_env_json",
			"version:1_json",
		},
		expectedServerTags: []string{
			"service:app1_json",
			"env:testing_env_json",
			"version:1_json",
			"http.iis.site:2",
			"http.iis.app_pool:DefaultAppPool",
			"'http.iis.sitename:TestSite1'",
		},
	},
	{
		name:            "test site with second application",
		description:     "Test different site in same IIS server, tests correct path discovery with an application",
		clientJSONFile:  "usmtest/client_all.json",
		clientAppConfig: "",
		defaultFiles: usmTaggingFiles{
			jsonFile:      "usmtest/defaultsite_all.json",
			appConfigFile: "usmtest/defaultsite_all.xml",
		},
		siteFiles: map[string]usmTaggingFiles{
			"TestSite1": {
				jsonFile: "usmtest/site1.json",
			},
		},
		appFiles: map[string]usmTaggingFiles{
			"/site1/app1": {
				jsonFile: "usmtest/site1_app1.json",
			},
			"/site1/app2": {
				jsonFile: "usmtest/site1_app2.json",
			},
			"/site1/app2/nested": {
				jsonFile: "usmtest/app2_nested.json",
			},
		},
		serverSiteName: "TestSite1", // empty is default site
		serverSitePort: "8081",
		targetPath:     "/site1/app2",
		expectedClientTags: []string{
			"service:webclient_json",
			"env:testing_env_json",
			"version:1_json",
		},
		expectedServerTags: []string{
			"service:app2_json",
			"env:testing_env_json",
			"version:1_json",
			"http.iis.site:2",
			"http.iis.app_pool:DefaultAppPool",
			"'http.iis.sitename:TestSite1'",
		},
	},
	{
		name:            "test site with nested application",
		description:     "Test different site in same IIS server, tests correct path discovery with an application",
		clientJSONFile:  "usmtest/client_all.json",
		clientAppConfig: "",
		defaultFiles: usmTaggingFiles{
			jsonFile:      "usmtest/defaultsite_all.json",
			appConfigFile: "usmtest/defaultsite_all.xml",
		},
		siteFiles: map[string]usmTaggingFiles{
			"TestSite1": {
				jsonFile: "usmtest/site1.json",
			},
		},
		appFiles: map[string]usmTaggingFiles{
			"/site1/app1": {
				jsonFile: "usmtest/site1_app1.json",
			},
			"/site1/app2": {
				jsonFile: "usmtest/site1_app2.json",
			},
			"/site1/app2/nested": {
				jsonFile: "usmtest/app2_nested.json",
			},
		},
		serverSiteName: "TestSite1", // empty is default site
		serverSitePort: "8081",
		targetPath:     "/site1/app2/nested",
		expectedClientTags: []string{
			"service:webclient_json",
			"env:testing_env_json",
			"version:1_json",
		},
		expectedServerTags: []string{
			"service:app2_nested_json",
			"env:testing_env_json",
			"version:1_json",
			"http.iis.site:2",
			"http.iis.app_pool:DefaultAppPool",
			"'http.iis.sitename:TestSite1'",
		},
	},
}
