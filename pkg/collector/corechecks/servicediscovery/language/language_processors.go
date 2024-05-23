// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package language

import (
	"bytes"
	"errors"
	"io"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language/reader"
)

func hasScript(r io.Reader, name string) bool {
	buf := make([]byte, 512)
	i, err := r.Read(buf)
	buf = buf[:i]
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	if !bytes.HasPrefix(buf, []byte{'#', '!'}) {
		return false
	}
	// clamp to first line
	pos := bytes.IndexByte(buf, '\n')
	if pos != -1 {
		buf = buf[:pos]
	}
	return bytes.Contains(buf, []byte(name))
}

// PythonScript is a Matcher for Python.
type PythonScript struct{}

// Match returns true if the language of the process is Python.
func (PythonScript) Match(pi ProcessInfo) bool {
	f, found := pi.FileReader()
	if !found {
		return false
	}
	defer f.Close()
	return hasScript(f, "python")
}

// Language returns the Language of the launching process
func (PythonScript) Language() Language {
	return Python
}

// RubyScript is a Matcher for Ruby.
type RubyScript struct{}

// Match returns true if the language of the process is Ruby.
func (RubyScript) Match(pi ProcessInfo) bool {
	f, found := pi.FileReader()
	if !found {
		return false
	}
	defer f.Close()
	return hasScript(f, "ruby")
}

// Language returns the Language of the launching process
func (RubyScript) Language() Language {
	return Ruby
}

// DotNetBinary is a Matcher for DotNet.
type DotNetBinary struct{}

// Match returns true if the language of the process is DotNet.
func (DotNetBinary) Match(pi ProcessInfo) bool {
	f, found := pi.FileReader()
	if !found {
		return false
	}
	defer f.Close()
	// scan the binary to see if it's a .net binary
	// as far as I know, all .net binaries have the string "DOTNET_ROOT" in them
	offset, err := reader.Index(f, "DOTNET_ROOT")
	return offset > -1 && err == nil
}

// Language returns the Language of the launching process
func (DotNetBinary) Language() Language {
	return DotNet
}
