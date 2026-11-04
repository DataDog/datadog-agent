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
	flag.Parse()

	if *srcPath == "" {
		if err := filterStdin(os.Stdout, flag.Args()); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := generate(*outPath, *srcPath, *goBin, *cc, *goroot, *testPath, *pkgName, flag.Args()); err != nil {
		log.Fatal(err)
	}
}

// filterStdin is the previous pipe interface: read cgo -godefs output on stdin.
// The test path has no .go suffix because `go run genpost.go <path>.go` would
// try to compile that path as another source file.
func filterStdin(w io.Writer, args []string) error {
	var buf bytes.Buffer
	if err := processFile(io.TeeReader(os.Stdin, &buf), w); err != nil {
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
// generated file). CC and GOROOT are made absolute first.
func generate(outPath, srcPath, goBin, cc, goroot, testPath, pkgName string, cgoFlags []string) error {
	if outPath == "" || goBin == "" || cc == "" || goroot == "" {
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
	cmd.Env = setEnv(setEnv(os.Environ(), "CC", abs(cc)), "GOROOT", abs(goroot))
	cmd.Stderr = os.Stderr
	raw, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("cgo -godefs: %w", err)
	}

	var processed bytes.Buffer
	if err := processFile(bytes.NewReader(raw), &processed); err != nil {
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

func processFile(rdr io.Reader, out io.Writer) error {
	b, err := io.ReadAll(rdr)
	if err != nil {
		return err
	}

	b = removeAbsolutePath(b, runtime.GOOS)

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

// removeAbsolutePath removes the absolute file path that is automatically output by cgo -godefs
// and replaces it with only the filename
func removeAbsolutePath(b []byte, platform string) []byte {
	var removeAbsolutePathRegex *regexp.Regexp
	switch platform {
	case "linux":
		removeAbsolutePathRegex = regexp.MustCompile(`(// cgo -godefs .+) /.+/([^/]+)$`)
	case "windows":
		removeAbsolutePathRegex = regexp.MustCompile(`(// cgo.exe -godefs .+) .:\\.+\\([^\\]+)$`)
	default:
		log.Fatal("unsupported platform")
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
