// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const repoModule = "github.com/DataDog/datadog-agent"

// loader parses Go packages on demand, without type-checking and ignoring
// build constraints: every .go file of a directory is part of the package.
// This makes the analysis independent of the build tags/GOOS used.
type loader struct {
	fset  *token.FileSet
	root  string // repository root
	pkgs  map[string]*pkgInfo
	names map[string]string // import path -> package name
}

type pkgInfo struct {
	dir     string
	module  string // directory of the closest go.mod
	files   []*fileInfo
	funcs   map[string][]*funcInfo
	methods map[string]map[string][]*funcInfo // receiver type -> method -> declarations
	embeds  map[string][]embedRef             // struct type -> embedded fields
	types   map[string]bool
	ifaces  map[string]bool     // interface types
	values  map[string]ast.Expr // package-level const/var -> value expression
}

type fileInfo struct {
	path    string
	ast     *ast.File
	imports map[string]string // local name -> import path
	pkg     *pkgInfo
}

type funcInfo struct {
	name string
	recv string
	decl *ast.FuncDecl
	file *fileInfo
}

type embedRef struct {
	file *fileInfo
	expr ast.Expr
}

type typeRef struct {
	pkg  *pkgInfo
	name string
}

func newLoader(root string) *loader {
	return &loader{
		fset:  token.NewFileSet(),
		root:  root,
		pkgs:  map[string]*pkgInfo{},
		names: map[string]string{},
	}
}

// findRepoRoot walks up from dir until it finds the datadog-agent root go.mod.
func findRepoRoot(dir string) (string, error) {
	for d := dir; ; d = filepath.Dir(d) {
		if readModule(filepath.Join(d, "go.mod")) == repoModule {
			return d, nil
		}
		if filepath.Dir(d) == d {
			return "", fmt.Errorf("%s is not inside the %s repository", dir, repoModule)
		}
	}
}

func readModule(gomod string) string {
	f, err := os.Open(gomod)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); strings.HasPrefix(line, "module ") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "module ")), `"`)
		}
	}
	return ""
}

func findModuleRoot(dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			return ""
		}
	}
}

// importDir maps an import path of this repository to its directory.
func (l *loader) importDir(path string) string {
	var rel string
	switch {
	case path == repoModule:
		rel = ""
	case strings.HasPrefix(path, repoModule+"/"):
		rel = strings.TrimPrefix(path, repoModule+"/")
	default:
		return ""
	}
	dir := filepath.Join(l.root, filepath.FromSlash(rel))
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return ""
	}
	return dir
}

var versionSuffix = regexp.MustCompile(`^v[0-9]+$`)

// pkgName returns the package name used when an import has no explicit alias.
func (l *loader) pkgName(path string) string {
	if n, ok := l.names[path]; ok {
		return n
	}
	name := ""
	if dir := l.importDir(path); dir != "" {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, e.Name()), nil, parser.PackageClauseOnly)
			if err == nil {
				name = f.Name.Name
				break
			}
		}
	}
	if name == "" {
		parts := strings.Split(path, "/")
		name = parts[len(parts)-1]
		if versionSuffix.MatchString(name) && len(parts) > 1 {
			name = parts[len(parts)-2]
		}
		if i := strings.Index(name, "."); i > 0 {
			name = name[:i]
		}
		name = strings.TrimPrefix(name, "go-")
		name = strings.ReplaceAll(name, "-", "")
	}
	l.names[path] = name
	return name
}

// load parses every Go file of dir. Test files are only included when withTests is set.
func (l *loader) load(dir string, withTests bool) *pkgInfo {
	key := fmt.Sprintf("%s|%t", dir, withTests)
	if p, ok := l.pkgs[key]; ok {
		return p
	}
	p := &pkgInfo{
		dir:     dir,
		module:  findModuleRoot(dir),
		funcs:   map[string][]*funcInfo{},
		methods: map[string]map[string][]*funcInfo{},
		embeds:  map[string][]embedRef{},
		types:   map[string]bool{},
		ifaces:  map[string]bool{},
		values:  map[string]ast.Expr{},
	}
	l.pkgs[key] = p

	entries, err := os.ReadDir(dir)
	if err != nil {
		return p
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		if !withTests && strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		f, err := parser.ParseFile(l.fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", path, err)
			continue
		}
		fi := &fileInfo{path: path, ast: f, imports: map[string]string{}, pkg: p}
		for _, imp := range f.Imports {
			ipath, _ := strconv.Unquote(imp.Path.Value)
			alias := ""
			if imp.Name != nil {
				alias = imp.Name.Name
				if alias == "_" || alias == "." {
					continue
				}
			} else {
				alias = l.pkgName(ipath)
			}
			fi.imports[alias] = ipath
		}
		p.files = append(p.files, fi)
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				info := &funcInfo{name: d.Name.Name, decl: d, file: fi}
				if d.Recv == nil || len(d.Recv.List) == 0 {
					p.funcs[info.name] = append(p.funcs[info.name], info)
					continue
				}
				info.recv = baseTypeName(d.Recv.List[0].Type)
				if p.methods[info.recv] == nil {
					p.methods[info.recv] = map[string][]*funcInfo{}
				}
				p.methods[info.recv][info.name] = append(p.methods[info.recv][info.name], info)
			case *ast.GenDecl:
				if d.Tok == token.CONST || d.Tok == token.VAR {
					for _, spec := range d.Specs {
						vs := spec.(*ast.ValueSpec)
						for i, n := range vs.Names {
							if i < len(vs.Values) && len(vs.Names) == len(vs.Values) {
								p.values[n.Name] = vs.Values[i]
							}
						}
					}
					continue
				}
				if d.Tok != token.TYPE {
					continue
				}
				for _, spec := range d.Specs {
					ts := spec.(*ast.TypeSpec)
					p.types[ts.Name.Name] = true
					if _, ok := ts.Type.(*ast.InterfaceType); ok {
						p.ifaces[ts.Name.Name] = true
					}
					var fields *ast.FieldList
					switch t := ts.Type.(type) {
					case *ast.StructType:
						fields = t.Fields
					case *ast.InterfaceType:
						fields = t.Methods
					default:
						continue
					}
					for _, field := range fields.List {
						if len(field.Names) == 0 {
							p.embeds[ts.Name.Name] = append(p.embeds[ts.Name.Name], embedRef{file: fi, expr: field.Type})
						}
					}
				}
			}
		}
	}
	return p
}

// baseTypeName strips pointers and generic instantiations from a receiver type.
func baseTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return baseTypeName(t.X)
	case *ast.ParenExpr:
		return baseTypeName(t.X)
	case *ast.IndexExpr:
		return baseTypeName(t.X)
	case *ast.IndexListExpr:
		return baseTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	}
	return ""
}

func stripType(expr ast.Expr) ast.Expr {
	for {
		switch t := expr.(type) {
		case *ast.StarExpr:
			expr = t.X
		case *ast.ParenExpr:
			expr = t.X
		case *ast.IndexExpr:
			expr = t.X
		case *ast.IndexListExpr:
			expr = t.X
		case *ast.Ellipsis:
			expr = t.Elt
		default:
			return expr
		}
	}
}

// resolveType resolves a type expression to a named type declared in this repository.
func (l *loader) resolveType(file *fileInfo, expr ast.Expr) *typeRef {
	switch t := stripType(expr).(type) {
	case *ast.Ident:
		if file.pkg.types[t.Name] {
			return &typeRef{pkg: file.pkg, name: t.Name}
		}
	case *ast.SelectorExpr:
		id, ok := t.X.(*ast.Ident)
		if !ok {
			return nil
		}
		dir := l.importDir(file.imports[id.Name])
		if dir == "" {
			return nil
		}
		if pkg := l.load(dir, false); pkg.types[t.Sel.Name] {
			return &typeRef{pkg: pkg, name: t.Sel.Name}
		}
	}
	return nil
}

// lookupMethod finds a method on a type or, following Go promotion rules, on its embedded fields.
func (l *loader) lookupMethod(tr *typeRef, name string) []*funcInfo {
	return l.lookupMethodDepth(tr, name, 0)
}

func (l *loader) lookupMethodDepth(tr *typeRef, name string, depth int) []*funcInfo {
	if tr == nil || depth > 10 {
		return nil
	}
	if m := tr.pkg.methods[tr.name][name]; len(m) > 0 {
		return m
	}
	for _, emb := range tr.pkg.embeds[tr.name] {
		if found := l.lookupMethodDepth(l.resolveType(emb.file, emb.expr), name, depth+1); len(found) > 0 {
			return found
		}
	}
	return nil
}

// isSuite reports whether a type embeds (directly or not) a testify suite.Suite
// or an e2e-framework suite, i.e. whether testify assertion methods are promoted to it.
func (l *loader) isSuite(tr *typeRef) bool {
	return l.isSuiteDepth(tr, 0)
}

func (l *loader) isSuiteDepth(tr *typeRef, depth int) bool {
	if tr == nil || depth > 10 {
		return false
	}
	if strings.HasSuffix(tr.pkg.dir, filepath.FromSlash("/test/e2e-framework/testing/e2e")) {
		return true
	}
	for _, emb := range tr.pkg.embeds[tr.name] {
		if sel, ok := stripType(emb.expr).(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok {
				path := emb.file.imports[id.Name]
				if path == "github.com/stretchr/testify/suite" || path == repoModule+"/test/e2e-framework/testing/e2e" {
					return true
				}
			}
		}
		if l.isSuiteDepth(l.resolveType(emb.file, emb.expr), depth+1) {
			return true
		}
	}
	return false
}

// methodSet returns the methods (own and promoted) of a type, in declaration order.
func (l *loader) methodSet(tr *typeRef) []*funcInfo {
	seen := map[string]bool{}
	var out []*funcInfo
	var visit func(tr *typeRef, depth int)
	visit = func(tr *typeRef, depth int) {
		if tr == nil || depth > 10 {
			return
		}
		var own []*funcInfo
		for name, decls := range tr.pkg.methods[tr.name] {
			if !seen[name] {
				seen[name] = true
				own = append(own, decls[0])
			}
		}
		l.sortFuncs(own)
		out = append(out, own...)
		for _, emb := range tr.pkg.embeds[tr.name] {
			visit(l.resolveType(emb.file, emb.expr), depth+1)
		}
	}
	visit(tr, 0)
	return out
}

func (l *loader) sortFuncs(fs []*funcInfo) {
	sort.Slice(fs, func(i, j int) bool {
		pi, pj := l.fset.Position(fs[i].decl.Pos()), l.fset.Position(fs[j].decl.Pos())
		if pi.Filename != pj.Filename {
			return pi.Filename < pj.Filename
		}
		return pi.Offset < pj.Offset
	})
}
