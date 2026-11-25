// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package trivy

import (
	"path"
	"strings"
	"time"

	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/sbom/core"
	"github.com/aquasecurity/trivy/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/sbom/usage"
)

// componentRefs returns the bom-refs of the components of bom, by the UID of
// their package. A package listed in several places shares its UID.
func componentRefs(bom *core.BOM) map[string][]string {
	refs := make(map[string][]string)
	for _, c := range bom.Components() {
		if uid, ref := c.PkgIdentifier.UID, c.PkgIdentifier.BOMRef; uid != "" && ref != "" {
			refs[uid] = append(refs[uid], ref)
		}
	}
	return refs
}

// buildUsageIndex returns the usage index of the SBOM with the serial number
// serial made of report, whose packages have the bom-refs refs before the
// conversion that convertedRef applies.
func buildUsageIndex(report *types.Report, refs map[string][]string, convertedRef func(string) string, serial string, scanStart, scanEnd time.Time) *usage.Index {
	var b usage.Builder
	for i := range report.Results {
		result := &report.Results[i]
		class := usage.ClassLanguage
		if result.Class == types.ClassOSPkg {
			class = usage.ClassOS
		}
		for j := range result.Packages {
			pkg := &result.Packages[j]
			anchors := usageAnchors(result, pkg)
			var purl string
			if pkg.Identifier.PURL != nil {
				purl = pkg.Identifier.PURL.String()
			}
			for _, ref := range refs[pkg.Identifier.UID] {
				if ref = convertedRef(ref); ref != "" {
					b.Add(usage.Component{BOMRef: ref, Class: class, PURL: purl}, anchors...)
				}
			}
		}
	}
	return b.Build(serial, scanStart, scanEnd)
}

// usageAnchors returns the paths through which a process uses pkg, a package
// of result: the files of an OS package, the files of a Python wheel that hold
// code, the package.json of a Node.js package, the gemspec of a Ruby gem, the
// archive that holds a Java package, the binary that a Go or Rust module was
// built into, and the file of a runtime. A package found in a lock file
// gets no anchor.
func usageAnchors(result *types.Result, pkg *ftypes.Package) []string {
	if result.Class == types.ClassOSPkg {
		return pkg.InstalledFiles
	}
	var anchors []string
	switch result.Type {
	case ftypes.PythonPkg:
		for _, p := range pkg.InstalledFiles {
			if holdsPythonCode(p) {
				anchors = append(anchors, p)
			}
		}
	case ftypes.NodePkg, ftypes.GemSpec, ftypes.PythonExecutable, ftypes.NodeJsExecutable, ftypes.PhpExecutable, ftypes.JavaExecutable:
		anchors = append(anchors, pkg.FilePath)
	case ftypes.Jar:
		anchors = append(anchors, outerArchive(pkg.FilePath))
	case ftypes.GoBinary, ftypes.RustBinary:
		anchors = append(anchors, result.Target)
	}
	if len(anchors) == 1 && anchors[0] == "" {
		return nil
	}
	return anchors
}

// holdsPythonCode reports whether p, a file a wheel installed, holds code that
// a process loads or runs: a module, a compiled extension, a shared library or
// a console script. Bytecode counts through its source, which the usage
// recorder reports beside it.
func holdsPythonCode(p string) bool {
	dir, name := path.Split(p)
	switch {
	case strings.HasSuffix(name, ".py"), strings.HasSuffix(name, ".pyd"),
		strings.HasSuffix(name, ".so"), strings.Contains(name, ".so."):
		return true
	case strings.HasSuffix(name, ".pyc"), strings.Contains(dir, ".dist-info/"):
		return false
	}
	return path.Base(dir) == "bin"
}

// archiveSuffixes end the names of the Java archives that a JVM opens.
var archiveSuffixes = []string{".jar/", ".war/", ".ear/", ".par/"}

// outerArchive returns the archive at the start of p, the path of a Java
// package, which can name an archive nested in another one, such as
// app.war/WEB-INF/lib/guava.jar: the JVM opens the outer archive.
func outerArchive(p string) string {
	end := len(p)
	for _, suffix := range archiveSuffixes {
		if i := strings.Index(p, suffix); i >= 0 && i+len(suffix)-1 < end {
			end = i + len(suffix) - 1
		}
	}
	return p[:end]
}
