// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package iisconfig

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/windows/registry"
)

/*
	 IIS can have multiple sites.  Each site can have multiple applications.
	   each application can have its own config.

	   Such as
	   <site name="app1" id="2">
	    <application path="/" applicationPool="app1">
	        <virtualDirectory path="/" physicalPath="C:\Temp" />
	    </application>
	    <application path="/app2" applicationPool="app1">
	        <virtualDirectory path="/" physicalPath="D:\temp" />
	    </application>
	    <application path="/app2/app3" applicationPool="app1">
	        <virtualDirectory path="/" physicalPath="D:\source" />
	    </application>


		in the above, there each application should be treated separately.

		so if theURL is /app2/app3/appx, then we look in d:\source
		                /app2/app4/appx, then we look in d:\tmp
						/app3            then we look in app1

	  pathTreeEntry implements a search tree to simplify the search for matching paths.
*/
type pathTreeEntry struct {
	nodes     map[string]*pathTreeEntry
	ddjson    APMTags
	appconfig APMTags
	envvars   APMTags
}

func splitPaths(path string) []string {
	path = filepath.Clean(path)
	if path == "/" || path == "\\" {
		return make([]string, 0)
	}
	a, b := filepath.Split(path)
	s := splitPaths(a)
	return append(s, b)
}

func findInPathTree(pathTrees map[uint32]*pathTreeEntry, siteID uint32, urlpath string) (APMTags, APMTags, APMTags) {
	// urlpath will come in as something like
	// /path/to/app

	// break down the path
	pathparts := splitPaths(urlpath)

	if _, ok := pathTrees[siteID]; !ok {
		return APMTags{}, APMTags{}, APMTags{}
	}
	if len(pathparts) == 0 {
		return pathTrees[siteID].ddjson, pathTrees[siteID].appconfig, pathTrees[siteID].envvars
	}

	currNode := pathTrees[siteID]

	for _, part := range pathparts {
		if _, ok := currNode.nodes[part]; !ok {
			return currNode.ddjson, currNode.appconfig, currNode.envvars
		}
		currNode = currNode.nodes[part]
	}
	return currNode.ddjson, currNode.appconfig, currNode.envvars
}

func addToPathTree(pathTrees map[uint32]*pathTreeEntry, siteID string, urlpath string, ddjson, appconfig, envvars APMTags) {

	intid, err := strconv.Atoi(siteID)
	if err != nil {
		return
	}
	id := uint32(intid)
	// urlpath will come in as something like
	// /path/to/app
	// need to build the tree all the way down

	// break down the path
	pathparts := splitPaths(urlpath)

	if _, ok := pathTrees[id]; !ok {
		pathTrees[id] = &pathTreeEntry{
			nodes: make(map[string]*pathTreeEntry),
		}
	}
	if len(pathparts) == 0 {
		pathTrees[id].ddjson = ddjson
		pathTrees[id].appconfig = appconfig
		pathTrees[id].envvars = envvars
		return
	}

	currNode := pathTrees[id]

	for _, part := range pathparts {
		if _, ok := currNode.nodes[part]; !ok {
			currNode.nodes[part] = &pathTreeEntry{
				nodes: make(map[string]*pathTreeEntry),
			}
		}
		currNode = currNode.nodes[part]
	}
	currNode.ddjson = ddjson
	currNode.appconfig = appconfig
	currNode.envvars = envvars
}

// GetAPMTags returns the APM tags for the given siteID and URL path.
// It returns three sources, in increasing precedence within DynamicTags:
// datadog.json, web.config, and environment variables from
// applicationHost.config (applicationPoolDefaults, applicationPools, and
// per-application environmentVariables).
func (iiscfg *DynamicIISConfig) GetAPMTags(siteID uint32, urlpath string) (APMTags, APMTags, APMTags) {
	iiscfg.mux.Lock()
	defer iiscfg.mux.Unlock()
	return findInPathTree(iiscfg.pathTrees, siteID, urlpath)
}

// isEmpty reports whether t has no UST fields set.
func (t APMTags) isEmpty() bool {
	return t.DDService == "" && t.DDEnv == "" && t.DDVersion == ""
}

// overlayAPMTags returns base with any non-empty fields from override applied
// on top. Empty fields in override do not clear base values; IIS provides
// <remove>/<clear> for that, which is not yet parsed here.
func overlayAPMTags(base, override APMTags) APMTags {
	out := base
	if override.DDService != "" {
		out.DDService = override.DDService
	}
	if override.DDEnv != "" {
		out.DDEnv = override.DDEnv
	}
	if override.DDVersion != "" {
		out.DDVersion = override.DDVersion
	}
	return out
}

// apmTagsFromEnvVars extracts UST fields from an IIS environmentVariables list.
// Name matching is case-insensitive: Windows env var names are case-insensitive,
// so <add name="Dd_Service"> in applicationHost.config produces an env var that
// Environment.GetEnvironmentVariable("DD_SERVICE") resolves successfully in
// w3wp.exe.
func apmTagsFromEnvVars(vars []iisEnvVar) APMTags {
	var tags APMTags
	for _, v := range vars {
		switch strings.ToUpper(v.Name) {
		case "DD_SERVICE":
			tags.DDService = v.Value
		case "DD_ENV":
			tags.DDEnv = v.Value
		case "DD_VERSION":
			tags.DDVersion = v.Value
		}
	}
	return tags
}

// buildPoolEnvTags returns a map keyed by lowercased pool name to UST tags
// derived from applicationHost.config applicationPools. Per-pool
// environmentVariables overlay applicationPoolDefaults entries. IIS treats
// pool names case-insensitively, so the lookup side must also lowercase.
func buildPoolEnvTags(pools iisApplicationPools) (perPool map[string]APMTags, defaults APMTags) {
	defaults = apmTagsFromEnvVars(pools.Defaults.EnvVars.Adds)
	perPool = make(map[string]APMTags, len(pools.Pools))
	for _, p := range pools.Pools {
		perPool[strings.ToLower(p.Name)] = overlayAPMTags(defaults, apmTagsFromEnvVars(p.EnvVars.Adds))
	}
	return perPool, defaults
}

// poolEnvFor returns the pool-level env tags for app.AppPool, falling back to
// applicationPoolDefaults when the pool name is not explicitly declared (IIS
// still applies defaults to implicit / inherited pools).
func poolEnvFor(perPool map[string]APMTags, defaults APMTags, poolName string) APMTags {
	if env, ok := perPool[strings.ToLower(poolName)]; ok {
		return env
	}
	return defaults
}

func buildPathTagTree(xmlcfg *iisConfiguration) map[uint32]*pathTreeEntry {
	pathTrees := make(map[uint32]*pathTreeEntry)
	perPool, defaults := buildPoolEnvTags(xmlcfg.ApplicationHost.ApplicationPools)
	sitesDefaultPool := xmlcfg.ApplicationHost.SitesAppDefaults.AppPool

	for _, site := range xmlcfg.ApplicationHost.Sites {
		siteDefaultPool := site.AppDefaults.AppPool
		if siteDefaultPool == "" {
			siteDefaultPool = sitesDefaultPool
		}
		for _, app := range site.Applications {
			// applicationHost.config exposes environmentVariables at the
			// application pool level (under <applicationPools><add> and
			// <applicationPoolDefaults>). Real per-application overrides
			// live under <location path="Site/App"><system.webServer>
			// <aspNetCore><environmentVariables>, which is not parsed here.
			// When the application omits applicationPool, IIS inherits it
			// from <site><applicationDefaults> or <sites><applicationDefaults>.
			appPool := app.AppPool
			if appPool == "" {
				appPool = siteDefaultPool
			}
			envvars := poolEnvFor(perPool, defaults, appPool)
			hasenv := !envvars.isEmpty()

			var ddjson APMTags
			var appconfig APMTags
			hasddjson := false
			haswebcfg := false

			for _, vdir := range app.VirtualDirs {
				if vdir.Path != "/" {
					// assume that non `/` virtual paths mean that
					// it's a virtual directory and not an application
					// the application root will always be at /
					continue
				}

				// check to see if the datadog.json or web.config exists
				ppath := vdir.PhysicalPath
				ppath, err := registry.ExpandString(ppath)
				if err != nil {
					ppath = vdir.PhysicalPath
				}

				ddjsonpath := filepath.Join(ppath, "datadog.json")
				webcfg := filepath.Join(ppath, "web.config")

				if _, err := os.Stat(ddjsonpath); err == nil {
					hasddjson = true
				}
				if _, err := os.Stat(webcfg); err == nil {
					haswebcfg = true
				}

				// Read into temporaries so a parse failure does not
				// overwrite values from an earlier vdir with partial /
				// zero data.
				if hasddjson {
					parsed, perr := ReadDatadogJSON(ddjsonpath)
					if perr != nil {
						hasddjson = false
					} else {
						ddjson = parsed
					}
				}
				if haswebcfg {
					parsed, perr := ReadDotNetConfig(webcfg)
					if perr != nil {
						haswebcfg = false
					} else {
						appconfig = parsed
					}
				}
			}

			if !hasddjson && !haswebcfg && !hasenv {
				continue
			}

			addToPathTree(pathTrees, site.SiteID, app.Path, ddjson, appconfig, envvars)
		}
	}
	return pathTrees
}
