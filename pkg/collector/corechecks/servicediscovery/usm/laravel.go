// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"bufio"
	"io"
	"io/fs"
	"path"
	"regexp"
	"strings"
)

type laravelParser struct {
	ctx DetectionContext
}

func newLaravelParser(ctx DetectionContext) *laravelParser {
	return &laravelParser{ctx: ctx}
}

// GetLaravelAppName resolves the app name for a laravel application
func (l laravelParser) GetLaravelAppName(artisan string) string {
	laravelDir := path.Dir(artisan)
	if name, ok := l.getLaravelAppNameFromEnv(laravelDir); ok {
		return name
	} else if name, ok := l.getLaravelAppNameFromConfig(laravelDir); ok {
		return name
	}
	return "laravel"
}

func getFirstMatchFromRegex(pattern string, content []byte) (string, bool) {
	regex := regexp.MustCompile(pattern)
	match := regex.FindSubmatch(content)
	for _, m := range match[1:] {
		if len(m) > 0 {
			return string(m), true
		}
	}
	return "", false
}

func trimPrefixFromLine(fs fs.SubFS, file string, prefix string) (string, bool) {
	if f, err := fs.Open(file); err == nil {
		defer f.Close()
		scn := bufio.NewScanner(f)
		for scn.Scan() {
			if value, ok := strings.CutPrefix(scn.Text(), prefix); ok {
				return value, true
			}
		}
	}
	return "", false
}

func (l laravelParser) getLaravelAppNameFromEnv(laravelDir string) (string, bool) {
	envFileName := path.Join(laravelDir, ".env")
	if l.ctx.fs != nil {
		if name, ok := trimPrefixFromLine(l.ctx.fs, envFileName, "DD_SERVICE="); ok {
			return name, true
		} else if name, ok := trimPrefixFromLine(l.ctx.fs, envFileName, "OTEL_SERVICE_NAME="); ok {
			return name, true
		} else if name, ok := trimPrefixFromLine(l.ctx.fs, envFileName, "APP_NAME="); ok {
			return name, true
		}
	}
	return "", false
}

func (l laravelParser) getLaravelAppNameFromConfig(dir string) (string, bool) {
	configFileName := path.Join(dir, "config", "app.php")
	if l.ctx.fs != nil {
		if f, err := l.ctx.fs.Open(configFileName); err == nil {
			defer f.Close()
			configFileContent, err := io.ReadAll(f)
			if err != nil {
				return "", false
			}

			// Matches the likes of: env('APP_NAME','<myAppName>'), or 'name'=>"<myAppName>"
			if name, ok := getFirstMatchFromRegex(`env\(\s*["']APP_NAME["']\s*,\s*["'](.*?)["']\s*\)|["']name["']\s*=>\s*["'](.*?)["']`, configFileContent); ok {
				return name, true
			}
		}
	}
	return "", false
}
