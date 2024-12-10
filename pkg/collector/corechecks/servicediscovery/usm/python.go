// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"io/fs"
	"path"
	"path/filepath"
	"strings"
)

const (
	initPy             = "__init__.py"
	allPyFiles         = "*.py"
	gunicornEnvCmdArgs = "GUNICORN_CMD_ARGS"
	wsgiAppEnv         = "WSGI_APP"
)

type pythonDetector struct {
	ctx      DetectionContext
	gunicorn detector
}

type gunicornDetector struct {
	ctx DetectionContext
}

func newGunicornDetector(ctx DetectionContext) detector {
	return &gunicornDetector{ctx: ctx}
}

func newPythonDetector(ctx DetectionContext) detector {
	return &pythonDetector{ctx: ctx, gunicorn: newGunicornDetector(ctx)}
}

func (p pythonDetector) detect(args []string) (ServiceMetadata, bool) {
	// When Gunicorn is invoked via its wrapper script the command line ends up
	// looking like the example below, so redirect to the Gunicorn detector for
	// this case:
	//  /usr/bin/python3 /usr/bin/gunicorn foo:app()
	if len(args) > 0 && filepath.Base(args[0]) == "gunicorn" {
		return p.gunicorn.detect(args[1:])
	}

	var (
		prevArgIsFlag bool
		moduleFlag    bool
	)

	for _, a := range args {
		hasFlagPrefix, isEnvVariable := strings.HasPrefix(a, "-"), strings.ContainsRune(a, '=')

		shouldSkipArg := prevArgIsFlag || hasFlagPrefix || isEnvVariable

		if moduleFlag {
			return NewServiceMetadata(a, CommandLine), true
		}

		if !shouldSkipArg {
			wd, _ := workingDirFromEnvs(p.ctx.Envs)
			absPath := abs(a, wd)
			fi, err := fs.Stat(p.ctx.fs, absPath)
			if err != nil {
				return ServiceMetadata{}, false
			}
			stripped := absPath
			var filename string
			if !fi.IsDir() {
				stripped, filename = path.Split(stripped)
				// If the path is a root level file, return the filename
				if stripped == "" {
					return NewServiceMetadata(p.findNearestTopLevel(filename), CommandLine), true
				}
			}
			if value, ok := p.deducePackageName(path.Clean(stripped), filename); ok {
				return NewServiceMetadata(value, Python), true
			}

			name := p.findNearestTopLevel(stripped)
			// If we have generic/useless directory names, fallback to the filename.
			if name == "." || name == "/" || name == "bin" || name == "sbin" {
				name = p.findNearestTopLevel(filename)
			}

			return NewServiceMetadata(name, CommandLine), true
		}

		if hasFlagPrefix && a == "-m" {
			moduleFlag = true
		}

		// The -u (unbuffered) option doesn't take an argument so we should
		// consider the next arg even though this one is a flag.
		prevArgIsFlag = hasFlagPrefix && a != "-u"
	}

	return ServiceMetadata{}, false
}

// deducePackageName is walking until a `__init__.py` is not found. All the dir traversed are joined then with `.`
func (p pythonDetector) deducePackageName(fp string, fn string) (string, bool) {
	up := path.Dir(fp)
	current := fp
	var traversed []string
	for run := true; run; run = current != up {
		if _, err := fs.Stat(p.ctx.fs, path.Join(current, initPy)); err != nil {
			break
		}
		traversed = append([]string{path.Base(current)}, traversed...)
		current = up
		up = path.Dir(current)
	}
	if len(traversed) > 0 && len(fn) > 0 {
		traversed = append(traversed, strings.TrimSuffix(fn, path.Ext(fn)))
	}
	return strings.Join(traversed, "."), len(traversed) > 0

}

// findNearestTopLevel returns the top level dir the contains a .py file starting walking up from fp.
// If fp is a file, it returns the filename without the extension.
func (p pythonDetector) findNearestTopLevel(fp string) string {
	up := path.Dir(fp)
	current := fp
	last := current
	for run := true; run; run = current != up {
		if matches, err := fs.Glob(p.ctx.fs, path.Join(current, allPyFiles)); err != nil || len(matches) == 0 {
			break
		}
		last = current
		current = up
		up = path.Dir(current)
	}
	filename := path.Base(last)
	return strings.TrimSuffix(filename, path.Ext(filename))
}

func (g gunicornDetector) detect(args []string) (ServiceMetadata, bool) {
	if fromEnv, ok := extractEnvVar(g.ctx.Envs, gunicornEnvCmdArgs); ok {
		name, ok := extractGunicornNameFrom(strings.Split(fromEnv, " "))
		if ok {
			return NewServiceMetadata(name, Gunicorn), true
		}
	}
	if wsgiApp, ok := extractEnvVar(g.ctx.Envs, wsgiAppEnv); ok && len(wsgiApp) > 0 {
		return NewServiceMetadata(parseNameFromWsgiApp(wsgiApp), Gunicorn), true
	}

	if name, ok := extractGunicornNameFrom(args); ok {
		// gunicorn replaces the cmdline with something like "gunicorn: master
		// [package]", so strip out the square brackets.
		name = strings.Trim(name, "[]")
		return NewServiceMetadata(name, CommandLine), true
	}
	return NewServiceMetadata("gunicorn", CommandLine), true
}

func extractGunicornNameFrom(args []string) (string, bool) {
	skip, capture := false, false
	for _, a := range args {
		if capture {
			return a, true
		}
		if skip {
			skip = false
			continue
		}
		if strings.HasPrefix(a, "-") {
			if a == "-n" {
				capture = true
				continue
			}
			skip = !strings.ContainsRune(a, '=')
			if skip {
				continue
			}
			if value, ok := strings.CutPrefix(a, "--name="); ok {
				return value, true
			}
		} else {
			return parseNameFromWsgiApp(args[len(args)-1]), true
		}
	}
	return "", false
}

func parseNameFromWsgiApp(wsgiApp string) string {
	name, _, _ := strings.Cut(wsgiApp, ":")
	return name
}
