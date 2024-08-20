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

	"golang.org/x/sys/windows/registry"
)

type pathTreeEntry struct {
	nodes     map[string]*pathTreeEntry
	ddjson    APMTags
	appconfig APMTags
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

func findInPathTree(pathtrees map[uint32]*pathTreeEntry, id uint32, urlpath string) (APMTags, APMTags) {
	// urlpath will come in as something like
	// /path/to/app
	// need to build the tree all the way down

	// break down the path
	pathparts := splitPaths(urlpath)

	if _, ok := pathtrees[id]; !ok {
		return APMTags{}, APMTags{}
	}
	if len(pathparts) == 0 {
		return pathtrees[id].ddjson, pathtrees[id].appconfig
	}

	currNode := pathtrees[id]

	for _, part := range pathparts {
		if _, ok := currNode.nodes[part]; !ok {
			return currNode.ddjson, currNode.appconfig
		}
		currNode = currNode.nodes[part]
	}
	return currNode.ddjson, currNode.appconfig
}

func addToPathTree(pathtrees map[uint32]*pathTreeEntry, siteID string, urlpath string, ddjson, appconfig APMTags) {

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

	if _, ok := pathtrees[id]; !ok {
		pathtrees[id] = &pathTreeEntry{
			nodes: make(map[string]*pathTreeEntry),
		}
	}
	if len(pathparts) == 0 {
		pathtrees[id].ddjson = ddjson
		pathtrees[id].appconfig = appconfig
		return
	}

	currNode := pathtrees[id]

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
	return

}
func (iiscfg *DynamicIISConfig) GetAPMTags(siteID uint32, urlpath string) (APMTags, APMTags) {
	iiscfg.mux.Lock()
	defer iiscfg.mux.Unlock()
	return findInPathTree(iiscfg.pathtrees, siteID, urlpath)
}

func buildPathTagTree(xmlcfg *iisConfiguration) map[uint32]*pathTreeEntry {
	pathtrees := make(map[uint32]*pathTreeEntry)

	for _, site := range xmlcfg.ApplicationHost.Sites {
		for _, app := range site.Applications {
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

				ddjsonpath := filepath.Join(ppath, "datadog.json")
				webcfg := filepath.Join(ppath, "web.config")
				hasddjson := false
				haswebcfg := false
				var ddjson APMTags
				var appconfig APMTags

				if _, err := os.Stat(ddjsonpath); err == nil {
					hasddjson = true
				}
				if _, err := os.Stat(webcfg); err == nil {
					haswebcfg = true
				}

				if hasddjson {
					ddjson, err = ReadDatadogJson(ddjsonpath)
					if err != nil {
						hasddjson = false
					}
				}
				if haswebcfg {
					appconfig, err = ReadDotNetConfig(webcfg)
					if err != nil {
						haswebcfg = false
					}
				}
				if !hasddjson && !haswebcfg {
					continue
				}

				addToPathTree(pathtrees, site.SiteID, app.Path, ddjson, appconfig)
			}
		}
	}
	return pathtrees
}
