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

type argType int

const (
	argNone argType = iota
	argMod
	argFileName
)

// parsePythonArgs parses the CPython command line arguments to find the module
// name or the file name. For this, besides handling the -m option to get the
// module name, we need to specifically handle any option that is known to take
// an argument so that we can skip the argument and not misinterpret it as a
// filename.
//
// We assume that all the other options other than the ones explicitly handled
// below do not take any arguments.
func parsePythonArgs(args []string) (argType, string) {
	skipNext := false
	modNext := false
	for _, arg := range args {
		if modNext {
			return argMod, arg
		}

		if skipNext {
			skipNext = false
			continue
		}

		if strings.HasPrefix(arg, "--") {
			// Only long arg with an argument.  CPython doesn't allow
			// including the argument with an equals sign in the same arg.
			if arg == "--check-hash-based-pycs" {
				skipNext = true
			}
		} else if strings.HasPrefix(arg, "-") {
		INNER:
			for charidx, char := range arg[1:] {
				rest := arg[1+charidx+1:]
				switch char {
				case 'c':
					// Everything after -c is a command and it terminates
					// the option parsing.
					return argNone, ""
				case 'm':
					// Module name, either attached here or in the next arg.
					if len(rest) > 0 {
						return argMod, rest
					}

					modNext = true
				case 'X', 'W':
					// Takes an argument, either attached here or in the next arg.
					if len(rest) > 0 {
						break INNER
					}

					skipNext = true
				}
			}
		} else {
			return argFileName, arg
		}
	}
	return argNone, ""
}

func (p pythonDetector) detect(args []string) (ServiceMetadata, bool) {
	// When Gunicorn is invoked via its wrapper script the command line ends up
	// looking like the example below, so redirect to the Gunicorn detector for
	// this case:
	//  /usr/bin/python3 /usr/bin/gunicorn foo:app()
	//
	// Another case where we want to redirect to the Gunicorn detector is when
	// gunicorn replaces its command line with something like the below. Because
	// of the [ready], we end up here first instead of going directly to the
	// Gunicorn detector.
	//  [ready] gunicorn: worker [airflow-webserver]
	if len(args) > 0 {
		base := filepath.Base(args[0])
		if base == "gunicorn" || base == "gunicorn:" {
			return p.gunicorn.detect(args[1:])
		}
		if base == "uvicorn" {
			return detectUvicorn(args[1:])
		}
	}

	argType, arg := parsePythonArgs(args)
	switch argType {
	case argNone:
		return ServiceMetadata{}, false
	case argMod:
		return NewServiceMetadata(arg, CommandLine), true
	case argFileName:
		absPath := p.ctx.resolveWorkingDirRelativePath(arg)
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
		return NewServiceMetadata(name, CommandLine), true
	}
	return NewServiceMetadata("gunicorn", CommandLine), true
}

func extractGunicornNameFrom(args []string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}

	// gunicorn sometimes replaces the cmdline with something like "gunicorn: master [package]".
	lastArg := args[len(args)-1]
	if len(lastArg) >= 2 && lastArg[0] == '[' && lastArg[len(lastArg)-1] == ']' {
		// Extract the name between the brackets
		name := lastArg[1 : len(lastArg)-1]
		return parseNameFromWsgiApp(name), true
	}

	// If the command line is not replaced, we need to parse the arguments.
	// Prefer the --name argument if one exists, otherwise try to find the first
	// non-flag argument.

	// List of long options that do NOT take an argument.  This list is shorter
	// than the ones which do take an argument.
	noArgOptions := map[string]struct{}{
		"--reload":                            {},
		"--spew":                              {},
		"--check-config":                      {},
		"--print-config":                      {},
		"--preload":                           {},
		"--no-sendfile":                       {},
		"--reuse-port":                        {},
		"--daemon":                            {},
		"--initgroups":                        {},
		"--capture-output":                    {},
		"--log-syslog":                        {},
		"--enable-stdio-inheritance":          {},
		"--disable-redirect-access-to-syslog": {},
		"--proxy-protocol":                    {},
		"--suppress-ragged-eofs":              {},
		"--do-handshake-on-connect":           {},
		"--strip-header-spaces":               {},
	}

	nameNext := false
	skipNext := false
	candidate := ""

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if nameNext {
			return arg, true
		}
		if skipNext {
			skipNext = false
			continue
		}

		if strings.HasPrefix(arg, "--name=") {
			// --name=VALUE
			return arg[len("--name="):], true
		} else if arg == "--name" {
			// --name VALUE
			nameNext = true
		} else if strings.HasPrefix(arg, "--") {
			// Skip --option=value
			if strings.Contains(arg, "=") {
				continue
			}
			// Skip the next argument unless we know that the option does not
			// take an argument
			if _, ok := noArgOptions[arg]; !ok {
				skipNext = true
			}
		} else if strings.HasPrefix(arg, "-") {
			// Single letter flags can be grouped together, e.g. "-Rnfoo"
		INNER:
			for charidx, char := range arg[1:] {
				rest := arg[1+charidx+1:]
				switch char {
				case 'n':
					if len(rest) > 0 {
						// Argument attached here
						return rest, true
					}
					// Argument in separate arg
					nameNext = true
				case 'R', 'd':
					// These are the only single letter flags that do not take an argument
					continue
				default:
					// Flag with argument
					if len(rest) > 0 {
						// Argument attached here
						break INNER
					}

					// Argument in separate arg
					skipNext = true
				}
			}
		} else if candidate == "" {
			candidate = arg
		}
	}

	if len(candidate) > 0 {
		// We didn't find a name flag, so try to parse the name from the first
		// potential app/module name.
		return parseNameFromWsgiApp(candidate), true
	}

	return "", false
}

func parseNameFromWsgiApp(wsgiApp string) string {
	name, _, _ := strings.Cut(wsgiApp, ":")
	return name
}

func detectUvicorn(args []string) (ServiceMetadata, bool) {
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}

		// Skip flags
		if strings.HasPrefix(arg, "-") {
			if arg == "--header" {
				// This takes an argument which looks similar to a module:app
				// pattern so skip that.
				skipNext = true
			}
			continue
		}

		// Look for module:app pattern
		if strings.Contains(arg, ":") {
			module := strings.Split(arg, ":")[0]
			return NewServiceMetadata(module, CommandLine), true
		}
	}
	return NewServiceMetadata("uvicorn", CommandLine), true
}
