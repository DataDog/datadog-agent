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

func findInPathTree(pathTrees map[uint32]*pathTreeEntry, siteId uint32, urlpath string) (APMTags, APMTags) {
	// urlpath will come in as something like
	// /path/to/app

	// break down the path
	pathparts := splitPaths(urlpath)

	if _, ok := pathTrees[siteId]; !ok {
		return APMTags{}, APMTags{}
	}
	if len(pathparts) == 0 {
		return pathTrees[siteId].ddjson, pathTrees[siteId].appconfig
	}

	currNode := pathTrees[siteId]

	for _, part := range pathparts {
		if _, ok := currNode.nodes[part]; !ok {
			return currNode.ddjson, currNode.appconfig
		}
		currNode = currNode.nodes[part]
	}
	return currNode.ddjson, currNode.appconfig
}

func addToPathTree(pathTrees map[uint32]*pathTreeEntry, siteID string, urlpath string, ddjson, appconfig APMTags) {

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
}
func (iiscfg *DynamicIISConfig) GetAPMTags(siteID uint32, urlpath string) (APMTags, APMTags) {
	iiscfg.mux.Lock()
	defer iiscfg.mux.Unlock()
	return findInPathTree(iiscfg.pathTrees, siteID, urlpath)
}

func buildPathTagTree(xmlcfg *iisConfiguration) map[uint32]*pathTreeEntry {
	pathTrees := make(map[uint32]*pathTreeEntry)

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
				if err != nil {
					ppath = vdir.PhysicalPath
				}

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
					ddjson, err = ReadDatadogJSON(ddjsonpath)
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

				addToPathTree(pathTrees, site.SiteID, app.Path, ddjson, appconfig)
			}
		}
	}
	return pathTrees
}
