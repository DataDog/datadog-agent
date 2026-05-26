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

// iisDefaultAppPoolName is the hard-coded pool name IIS assigns to an
// <application> that omits applicationPool when no <applicationDefaults>
// supplies one.
const iisDefaultAppPoolName = "DefaultAppPool"

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

// applyEnvVarsOver returns base with each <environmentVariables> directive
// applied in document order: <clear/> resets to empty, <remove name="X"/>
// wipes the named field, and <add name="X" value="V"/> sets it. Name
// matching is case-insensitive because Windows treats env var names that
// way: <add name="Dd_Service"> in applicationHost.config yields an env var
// that Environment.GetEnvironmentVariable("DD_SERVICE") resolves in
// w3wp.exe. Names other than DD_SERVICE/DD_ENV/DD_VERSION are ignored.
func applyEnvVarsOver(base APMTags, vars iisEnvironmentVariables) APMTags {
	state := base
	for _, op := range vars.Ops {
		switch op.kind {
		case iisEnvVarOpClear:
			state = APMTags{}
		case iisEnvVarOpRemove:
			switch strings.ToUpper(op.name) {
			case "DD_SERVICE":
				state.DDService = ""
			case "DD_ENV":
				state.DDEnv = ""
			case "DD_VERSION":
				state.DDVersion = ""
			}
		case iisEnvVarOpAdd:
			switch strings.ToUpper(op.name) {
			case "DD_SERVICE":
				state.DDService = op.value
			case "DD_ENV":
				state.DDEnv = op.value
			case "DD_VERSION":
				state.DDVersion = op.value
			}
		}
	}
	return state
}

// buildPoolEnvTags returns a map keyed by lowercased pool name to UST tags
// derived from applicationHost.config applicationPools. Per-pool
// environmentVariables directives are applied in document order on top of
// the resolved applicationPoolDefaults state. IIS treats pool names
// case-insensitively, so the lookup side must also lowercase.
func buildPoolEnvTags(pools iisApplicationPools) (perPool map[string]APMTags, defaults APMTags) {
	defaults = applyEnvVarsOver(APMTags{}, pools.Defaults.EnvVars)
	perPool = make(map[string]APMTags, len(pools.Pools))
	for _, p := range pools.Pools {
		perPool[strings.ToLower(p.Name)] = applyEnvVarsOver(defaults, p.EnvVars)
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

// buildLocationEnvOps returns a map keyed by lowercased <location> path to
// the ordered <environmentVariables> ops parsed from that location's
// <system.webServer><aspNetCore>. IIS matches location paths
// case-insensitively, so the lookup side must lowercase too.
func buildLocationEnvOps(locations []iisLocation) map[string][]iisEnvVarOp {
	out := make(map[string][]iisEnvVarOp, len(locations))
	for _, loc := range locations {
		if loc.Path == "" {
			continue
		}
		ops := loc.SystemWebServer.AspNetCore.EnvVars.Ops
		if len(ops) == 0 {
			continue
		}
		key := strings.ToLower(strings.Trim(loc.Path, "/"))
		// Later definitions for the same path replace earlier ones, matching
		// the last-wins semantics IIS applies when applicationHost.config
		// declares the same <location path> twice.
		out[key] = ops
	}
	return out
}

// applyLocationEnvOverlay walks the <location> hierarchy from least to most
// specific (site root, then each application path segment) and overlays each
// matching block's env var ops on top of base. This mirrors how the IIS
// configuration system inherits <location> blocks into nested paths, so an
// app-level <location path="Site/App"> can build on -- or override via
// <clear/>/<remove> -- a site-level <location path="Site">.
func applyLocationEnvOverlay(base APMTags, locOps map[string][]iisEnvVarOp, siteName, appPath string) APMTags {
	if len(locOps) == 0 {
		return base
	}
	state := base
	prefix := strings.ToLower(siteName)
	if ops, ok := locOps[prefix]; ok {
		state = applyEnvVarsOver(state, iisEnvironmentVariables{Ops: ops})
	}
	cleaned := strings.Trim(appPath, "/")
	if cleaned == "" {
		return state
	}
	for _, seg := range strings.Split(cleaned, "/") {
		if seg == "" {
			continue
		}
		prefix = prefix + "/" + strings.ToLower(seg)
		if ops, ok := locOps[prefix]; ok {
			state = applyEnvVarsOver(state, iisEnvironmentVariables{Ops: ops})
		}
	}
	return state
}

func buildPathTagTree(xmlcfg *iisConfiguration) map[uint32]*pathTreeEntry {
	pathTrees := make(map[uint32]*pathTreeEntry)
	perPool, defaults := buildPoolEnvTags(xmlcfg.ApplicationHost.ApplicationPools)
	sitesDefaultPool := xmlcfg.ApplicationHost.SitesAppDefaults.AppPool
	locationOps := buildLocationEnvOps(xmlcfg.Locations)

	for _, site := range xmlcfg.ApplicationHost.Sites {
		siteDefaultPool := site.AppDefaults.AppPool
		if siteDefaultPool == "" {
			siteDefaultPool = sitesDefaultPool
		}
		for _, app := range site.Applications {
			// applicationHost.config exposes environmentVariables at two
			// scopes: the application pool (under <applicationPools><add>
			// and <applicationPoolDefaults>) and the application itself
			// (under <location path="Site/App"><system.webServer>
			// <aspNetCore><environmentVariables>). Pool env vars resolve
			// first, then location overlays apply in increasing specificity
			// so an app-level <location> can override the pool value or
			// drop it via <clear/>/<remove name="..."/>.
			//
			// When the application omits applicationPool, IIS inherits it
			// from <site><applicationDefaults>, then <sites><applicationDefaults>,
			// and finally hard-codes "DefaultAppPool" if neither is set.
			appPool := app.AppPool
			if appPool == "" {
				appPool = siteDefaultPool
			}
			if appPool == "" {
				appPool = iisDefaultAppPoolName
			}
			envvars := poolEnvFor(perPool, defaults, appPool)
			envvars = applyLocationEnvOverlay(envvars, locationOps, site.Name, app.Path)
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
