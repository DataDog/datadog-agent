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

// iisDefaultAppPoolName is the pool IIS uses for an <application> with no
// applicationPool and no <applicationDefaults>.
const iisDefaultAppPoolName = "DefaultAppPool"

// pathTreeEntry is a per-site tree of application paths; each declared
// <application> is a node holding that app's resolved UST tags.
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

// applyEnvVarsOver applies the ops over base in document order (<clear/> resets,
// <remove> wipes, <add> sets); env var names match case-insensitively.
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

// buildPoolEnvTags maps lowercased pool name -> UST tags. A pool with its own
// <environmentVariables> replaces applicationPoolDefaults; one that omits it inherits.
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

// poolEnvFor returns the pool's env tags, falling back to applicationPoolDefaults
// for undeclared/implicit pools.
func poolEnvFor(perPool map[string]APMTags, defaults APMTags, poolName string) APMTags {
	if env, ok := perPool[strings.ToLower(poolName)]; ok {
		return env
	}
	return defaults
}

// buildLocationEnvOps maps lowercased <location> path -> ordered aspNetCore env
// ops. Key "" is the global overlay (pathless <location> + root <system.webServer>).
func buildLocationEnvOps(xmlcfg *iisConfiguration) map[string][]iisEnvVarOp {
	out := make(map[string][]iisEnvVarOp, len(xmlcfg.Locations)+1)
	// Root <system.webServer> ops first, then pathless <location> ops append.
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
		// Last definition for a path wins (IIS last-wins on duplicate <location>).
		out[key] = ops
	}
	if len(global) > 0 {
		out[""] = global
	}
	return out
}

// applyLocationEnvOverlay builds the effective aspNetCore env from empty (global,
// then site, then app-path segments) and overlays its non-empty fields onto base.
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
	return base.Overlay(aspNetCore)
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
			// Pool env resolves first; app <location> aspNetCore overlays it. An
			// omitted applicationPool inherits site, then sites, defaults, then DefaultAppPool.
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
			var webcfgEnv APMTags
			hasddjson := false
			haswebcfg := false

			for _, vdir := range app.VirtualDirs {
				if vdir.Path != "/" {
					// Non-"/" virtual paths are virtual directories, not the app root.
					continue
				}

				// Reset per vdir so a later root vdir without these files
				// doesn't keep an earlier one's stale state.
				hasddjson = false
				haswebcfg = false

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

				// Use temporaries so a parse failure doesn't overwrite an earlier vdir's data.
				if hasddjson {
					parsed, perr := ReadDatadogJSON(ddjsonpath)
					if perr != nil {
						hasddjson = false
					} else {
						ddjson = parsed
					}
				}
				if haswebcfg {
					envTags, appSettingsTags, perr := ReadDotNetConfig(webcfg)
					if perr != nil {
						haswebcfg = false
					} else {
						appconfig = appSettingsTags
						webcfgEnv = envTags
					}
				}
			}

			// Core web.config <aspNetCore> env overrides the pool env (ANCM
			// applies it last); fold it in so it outranks applicationHost. For a
			// Framework app webcfgEnv is empty, so this is a no-op.
			envvars = envvars.Overlay(webcfgEnv)

			// Add a node for every <application> (a worker boundary) even if empty,
			// so a child worker with no DD_* env doesn't inherit the parent's tags.
			addToPathTree(pathTrees, site.SiteID, app.Path, ddjson, appconfig, envvars)
		}
	}
	return pathTrees
}
