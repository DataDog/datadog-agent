// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main is the program to fixup cgo generated types
package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

func main() {
	outPath := flag.String("out", "", "file to write processed cgo output to")
	srcPath := flag.String("src", "", "cgo source file; genpost runs go tool cgo -godefs on it")
	goBin := flag.String("go", "", "go binary used to run cgo -godefs")
	cc := flag.String("cc", "", "C compiler passed to cgo as CC")
	goroot := flag.String("goroot", "", "GOROOT for the go binary")
	testPath := flag.String("test", "", "alignment test file to write")
	pkgName := flag.String("pkg", "", "package name for the alignment test")
	goos := flag.String("goos", runtime.GOOS, "GOOS the generated types are for")
	goarch := flag.String("goarch", runtime.GOARCH, "GOARCH the generated types are for")
	sysroot := flag.String("sysroot", "", "sysroot passed to the C compiler")
	var ccFlags stringList
	flag.Var(&ccFlags, "ccflag", "extra C compiler flag, passed verbatim and kept out of the generated header (repeatable)")
	flag.Parse()

	if *srcPath == "" {
		if err := filterStdin(os.Stdout, *goos, flag.Args()); err != nil {
			log.Fatal(err)
		}
		return
	}
	c := compiler{path: *cc, sysroot: *sysroot, flags: ccFlags}
	if err := generate(*outPath, *srcPath, *goBin, c, *goroot, *goos, *goarch, *testPath, *pkgName, flag.Args()); err != nil {
		log.Fatal(err)
	}
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, " ") }

func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

type compiler struct {
	path    string
	sysroot string
	flags   []string
}

// filterStdin is the previous pipe interface: read cgo -godefs output on stdin.
// The test path has no .go suffix because `go run genpost.go <path>.go` would
// try to compile that path as another source file.
func filterStdin(w io.Writer, goos string, args []string) error {
	var buf bytes.Buffer
	if err := processFile(io.TeeReader(os.Stdin, &buf), w, goos); err != nil {
		return err
	}
	if len(args) >= 2 && args[0] != "" && args[1] != "" {
		return writeTests(&buf, args[0]+".go", args[1])
	}
	return nil
}

// generate runs cgo -godefs and writes the processed output. Paths arrive
// relative to the execroot, but cgo's working directory is the source
// directory so that include flags stay relative (they are copied into the
// generated file). CC, its sysroot and GOROOT are made absolute first.
func generate(outPath, srcPath, goBin string, cc compiler, goroot, goos, goarch, testPath, pkgName string, cgoFlags []string) error {
	if outPath == "" || goBin == "" || cc.path == "" || goroot == "" {
		return errors.New("genpost requires -out, -go, -cc and -goroot")
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	abs := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(root, p)
	}

	srcAbs := abs(srcPath)
	argv := make([]string, 0, 5+len(cgoFlags))
	argv = append(argv, "tool", "cgo", "-godefs", "--")
	argv = append(argv, cgoFlags...)
	argv = append(argv, filepath.Base(srcAbs))

	cmd := exec.Command(abs(goBin), argv...)
	cmd.Dir = filepath.Dir(srcAbs)
	ccArgs := append([]string{abs(cc.path)}, cc.flags...)
	if cc.sysroot != "" {
		ccArgs = append(ccArgs, "--sysroot="+abs(cc.sysroot))
	}
	ccEnv, err := quoteArgs(ccArgs)
	if err != nil {
		return err
	}
	cmd.Env = os.Environ()
	for k, v := range map[string]string{"CC": ccEnv, "GOROOT": abs(goroot), "GOOS": goos, "GOARCH": goarch} {
		cmd.Env = setEnv(cmd.Env, k, v)
	}
	cmd.Stderr = os.Stderr
	raw, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("cgo -godefs: %w", err)
	}

	var processed bytes.Buffer
	if err := processFile(bytes.NewReader(raw), &processed, goos); err != nil {
		return err
	}
	if err := os.WriteFile(abs(outPath), processed.Bytes(), 0o644); err != nil {
		return err
	}
	if testPath == "" {
		return nil
	}
	if pkgName == "" {
		return errors.New("genpost requires -pkg with -test")
	}
	return writeTests(bytes.NewReader(raw), abs(testPath), pkgName)
}

func setEnv(env []string, key, val string) []string {
	prefix := key + "="
	for i, entry := range env {
		if len(entry) >= len(prefix) && strings.EqualFold(entry[:len(prefix)], prefix) {
			next := append([]string(nil), env...)
			next[i] = prefix + val
			return next
		}
	}
	return append(env, prefix+val)
}

// quoteArgs joins args into a value cgo splits back with cmd/internal/quoted.
// It has no escapes, so an argument holding both quote characters cannot be passed.
func quoteArgs(args []string) (string, error) {
	quoted := make([]string, len(args))
	for i, a := range args {
		switch {
		case !strings.ContainsAny(a, " \t\n'\""):
			quoted[i] = a
		case !strings.Contains(a, "'"):
			quoted[i] = "'" + a + "'"
		case !strings.Contains(a, `"`):
			quoted[i] = `"` + a + `"`
		default:
			return "", fmt.Errorf("cannot pass %q in CC: it contains both quote characters", a)
		}
	}
	return strings.Join(quoted, " "), nil
}

func processFile(rdr io.Reader, out io.Writer, goos string) error {
	b, err := io.ReadAll(rdr)
	if err != nil {
		return err
	}

	// The host's cgo wrote the path, so strip it in host style, and before
	// windowsCgoHeader renames "cgo": the POSIX pattern expects "// cgo -godefs".
	b = removeAbsolutePath(b, runtime.GOOS)
	b = windowsCgoHeader(b, goos)

	int8variableNames := []string{
		"Buf",
		"Cgroup",
		"Prev_cgroup_name",
		"Cgroup_name",
		"LocalAddr",
		"LocalAddress",
		"Probe_id",
		"RemoteAddr",
		"RemoteAddress",
		"Request_fragment",
		"Topic_name",
		"Trigger_comm",
		"Victim_comm",
		"Devices",
	}

	// Convert []int8 to []byte in multiple generated fields from the kernel, to simplify
	// conversion to string; see golang.org/issue/20753
	convertInt8ArrayToByteArrayRegex := regexp.MustCompile(`(` + strings.Join(int8variableNames, "|") + `)(\s+)\[(\d+)\]u?int8`)
	b = convertInt8ArrayToByteArrayRegex.ReplaceAll(b, []byte("$1$2[$3]byte"))

	// Convert generated pointers to CGo structs to uint64
	convertPointerToUint64Regex := regexp.MustCompile(`\*_Ctype_struct_(\w+)`)
	b = convertPointerToUint64Regex.ReplaceAll(b, []byte("uint64"))

	// Convert *byte and *uint64 pointers to uint64 (original void * in C)
	convertBytePointerToUint64Regex := regexp.MustCompile(`(\s+)(\*(byte|uint64))`)
	b = convertBytePointerToUint64Regex.ReplaceAll(b, []byte("${1}uint64"))

	b, err = format.Source(b)
	if err != nil {
		return err
	}

	_, err = out.Write(b)
	return err
}

var cgoHeaderRegex = regexp.MustCompile(`(?m)^// cgo -godefs `)

// windowsCgoHeader renames "cgo" to "cgo.exe" in Windows output: cgo writes its
// own binary name, so output generated on another OS would otherwise differ.
func windowsCgoHeader(b []byte, goos string) []byte {
	if goos != "windows" {
		return b
	}
	return cgoHeaderRegex.ReplaceAll(b, []byte("// cgo.exe -godefs "))
}

// removeAbsolutePath removes the absolute file path that is automatically output by cgo -godefs
// and replaces it with only the filename
func removeAbsolutePath(b []byte, platform string) []byte {
	removeAbsolutePathRegex := regexp.MustCompile(`(// cgo -godefs .+) /.+/([^/]+)$`)
	if platform == "windows" {
		removeAbsolutePathRegex = regexp.MustCompile(`(// cgo.exe -godefs .+) .:\\.+\\([^\\]+)$`)
	}
	return removeAbsolutePathRegex.ReplaceAll(b, []byte("$1 $2"))
}

var testHeaderTemplate = `// Code generated by genpost.go; DO NOT EDIT.

package %s
`

var testImportTemplate = `
import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
)
`

var testTemplate = `
func TestCgoAlignment_%[1]s(t *testing.T) {
	ebpftest.AssertCgoAlignment[%[1]s](t)
}
`

func writeTests(rdr io.Reader, dstFile string, packageName string) error {
	var typeNames []string
	structRegex := regexp.MustCompile("type ([^ ]+) struct")
	scanner := bufio.NewScanner(rdr)
	for scanner.Scan() {
		matches := structRegex.FindSubmatch(scanner.Bytes())
		if len(matches) >= 2 {
			typeName := matches[1]
			typeNames = append(typeNames, string(typeName))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	dst, err := os.Create(dstFile)
	if err != nil {
		return err
	}
	defer dst.Close()
	fmt.Fprintf(dst, testHeaderTemplate, packageName)

	if len(typeNames) > 0 {
		fmt.Fprint(dst, testImportTemplate)
		for _, typeName := range typeNames {
			fmt.Fprintf(dst, testTemplate, typeName)
		}
	}
	return nil
}
