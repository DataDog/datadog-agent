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

// buildPoolEnvTags maps lowercased pool name -> UST tags from
// applicationHost.config (pool names are case-insensitive). IIS does not merge a
// pool's own <environmentVariables> with applicationPoolDefaults: declaring the
// collection (even empty) replaces the defaults; only a pool that omits it
// inherits them.
func buildPoolEnvTags(pools iisApplicationPools) (perPool map[string]APMTags, defaults APMTags) {
	defaults = applyEnvVarsOver(APMTags{}, pools.Defaults.EnvVars)
	perPool = make(map[string]APMTags, len(pools.Pools))
	for _, p := range pools.Pools {
		if p.EnvVars.XMLName.Local != "" {
			perPool[strings.ToLower(p.Name)] = applyEnvVarsOver(APMTags{}, p.EnvVars)
		} else {
			perPool[strings.ToLower(p.Name)] = defaults
		}
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
//
// The empty key "" holds the "global" overlay: ops from any pathless
// <location> block and from the root <system.webServer><aspNetCore>. IIS
// inherits both into every site/application, so they form the base of the
// effective aspNetCore collection before site/app overlays apply.
func buildLocationEnvOps(xmlcfg *iisConfiguration) map[string][]iisEnvVarOp {
	out := make(map[string][]iisEnvVarOp, len(xmlcfg.Locations)+1)
	// Root <system.webServer><aspNetCore> ops come first in document order
	// among the global sources; pathless <location> ops layer on top via
	// append, and stay subject to the same last-wins rule as any other path.
	var global []iisEnvVarOp
	global = append(global, xmlcfg.SystemWebServer.AspNetCore.EnvVars.Ops...)
	for _, loc := range xmlcfg.Locations {
		ops := loc.SystemWebServer.AspNetCore.EnvVars.Ops
		if len(ops) == 0 {
			continue
		}
		if loc.Path == "" {
			global = append(global, ops...)
			continue
		}
		key := strings.ToLower(strings.Trim(loc.Path, "/"))
		// Later definitions for the same path replace earlier ones, matching
		// the last-wins semantics IIS applies when applicationHost.config
		// declares the same <location path> twice.
		out[key] = ops
	}
	if len(global) > 0 {
		out[""] = global
	}
	return out
}

// applyLocationEnvOverlay returns base with any <location><aspNetCore>
// <environmentVariables> values overlaid for (siteName, appPath).
//
// IIS evaluates these two scopes independently. applicationPools env vars are
// pushed into the w3wp.exe process environment when the worker starts.
// <aspNetCore> env vars are a separate IIS collection that the ASP.NET Core
// Module then adds on top of that process environment when launching the
// managed app -- they overwrite per key but cannot unset values the worker
// process already inherited. <clear/> and <remove> inside an aspNetCore block
// therefore operate on the aspNetCore collection itself (across <location>
// inheritance), not on the pool-resolved state.
//
// We mirror that by building the effective aspNetCore collection from empty,
// applying the global overlay (pathless <location> and root <system.webServer>
// at key ""), then each ancestor <location>'s ops in increasing specificity
// (site, then each app-path segment), and finally overlaying only the
// non-empty fields onto base. The .NET tracer reads from the merged process
// env, so agreeing on this order is what keeps USM tags aligned with tracer
// UST.
func applyLocationEnvOverlay(base APMTags, locOps map[string][]iisEnvVarOp, siteName, appPath string) APMTags {
	if len(locOps) == 0 {
		return base
	}
	aspNetCore := APMTags{}
	applied := false
	apply := func(key string) {
		if ops, ok := locOps[key]; ok {
			aspNetCore = applyEnvVarsOver(aspNetCore, iisEnvironmentVariables{Ops: ops})
			applied = true
		}
	}
	apply("")
	prefix := strings.ToLower(siteName)
	apply(prefix)
	cleaned := strings.Trim(appPath, "/")
	if cleaned != "" {
		for _, seg := range strings.Split(cleaned, "/") {
			if seg == "" {
				continue
			}
			prefix = prefix + "/" + strings.ToLower(seg)
			apply(prefix)
		}
	}
	if !applied {
		return base
	}
	result := base
	if aspNetCore.DDService != "" {
		result.DDService = aspNetCore.DDService
	}
	if aspNetCore.DDEnv != "" {
		result.DDEnv = aspNetCore.DDEnv
	}
	if aspNetCore.DDVersion != "" {
		result.DDVersion = aspNetCore.DDVersion
	}
	return result
}

func buildPathTagTree(xmlcfg *iisConfiguration) map[uint32]*pathTreeEntry {
	pathTrees := make(map[uint32]*pathTreeEntry)
	perPool, defaults := buildPoolEnvTags(xmlcfg.ApplicationHost.ApplicationPools)
	sitesDefaultPool := xmlcfg.ApplicationHost.SitesAppDefaults.AppPool
	locationOps := buildLocationEnvOps(xmlcfg)

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

			var ddjson APMTags
			var appconfig APMTags
			configFromEnv := false
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
					parsed, fromEnv, perr := ReadDotNetConfig(webcfg)
					if perr != nil {
						haswebcfg = false
					} else {
						appconfig = parsed
						configFromEnv = fromEnv
					}
				}
			}

			// A Core app's web.config <aspNetCore> env vars are the most
			// specific real env: ANCM overlays them over the pool env and the
			// tracer reads them at its top tier. Fold them onto the
			// applicationHost env (web.config wins per field) and drop the
			// separate tier so the result outranks applicationHost. Framework
			// <appSettings> ranks below real env, so it stays in appconfig.
			if configFromEnv {
				if appconfig.DDService != "" {
					envvars.DDService = appconfig.DDService
				}
				if appconfig.DDEnv != "" {
					envvars.DDEnv = appconfig.DDEnv
				}
				if appconfig.DDVersion != "" {
					envvars.DDVersion = appconfig.DDVersion
				}
				appconfig = APMTags{}
			}

			// Every declared <application> is a worker-process boundary, so
			// we add a tree node even when all three sources resolve empty.
			// findInPathTree returns the nearest ancestor's tags when a
			// segment is missing, so skipping an empty child would leak the
			// parent's pool tags onto requests served by a worker process
			// that has no DD_* env at all.
			addToPathTree(pathTrees, site.SiteID, app.Path, ddjson, appconfig, envvars)
		}
	}
	return pathTrees
}
