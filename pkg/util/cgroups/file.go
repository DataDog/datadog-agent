// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

var defaultFileReader = &osFileReader{}

type fileReader interface {
	open(path string) (file, error)
}

type file interface {
	io.Reader
	Close() error
}

type osFileReader struct{}

func (fr *osFileReader) open(path string) (file, error) {
	reportFileAccessed(path)
	return os.Open(path)
}

type stopParsingError struct{}

func (e *stopParsingError) Error() string {
	return "stopping file parsing" // should never be used
}

// returning an error will stop parsing and return the error
// with the exception of stopParsingError that will return without error
//
// the input slice will get overwritten, and should be copied if it needs to escape the scope of the parser.
type parser func([]byte) error

var readerPool = sync.Pool{New: func() any { return bufio.NewReader(nil) }}

func parseFile(fr fileReader, path string, p parser) error {
	file, err := fr.open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return readFile(file, p)
}

func readFile(file io.Reader, p parser) error {
	reader := readerPool.Get().(*bufio.Reader)
	reader.Reset(file)
	defer readerPool.Put(reader)

	for {
		line, isPrefix, err := reader.ReadLine()
		if err == io.EOF {
			break
		}

		// less efficient path, only if the line length exceeds the readers buffer
		if isPrefix {
			// TODO: test
			var accum []byte
			accum = append(accum, line...)
			for isPrefix {
				line, isPrefix, _ = reader.ReadLine()
				accum = append(accum, line...)
			}
			line = accum
		}

		err = p(line)
		if err != nil {
			if errors.Is(err, &stopParsingError{}) {
				return nil
			}
			return err
		}
	}
	return nil
}

func parseSingleSignedStat(fr fileReader, path string, val **int64) error {
	return parseFile(fr, path, func(line []byte) error {
		// handle cgroupv2 max value, we usually consider max == no value (limit)
		if bytes.Equal(line, []byte("max")) {
			return &stopParsingError{}
		}

		value, err := strconv.ParseInt(string(line), 10, 64)
		if err != nil {
			return newValueError(string(line), err)
		}
		*val = &value
		return &stopParsingError{}
	})
}

func parseSingleUnsignedStat(fr fileReader, path string, val **uint64) error {
	return parseFile(fr, path, func(line []byte) error {
		// handle cgroupv2 max value, we usually consider max == no value (limit)
		if bytes.Equal(line, []byte("max")) {
			return &stopParsingError{}
		}

		value, err := strconv.ParseUint(string(line), 10, 64)
		if err != nil {
			return newValueError(string(line), err)
		}
		*val = &value
		return &stopParsingError{}
	})
}

func parseColumnStats(fr fileReader, path string, valueParser func([]string) error) error {
	err := parseFile(fr, path, func(line []byte) error {
		splits := strings.Fields(string(line))
		return valueParser(splits)
	})

	return err
}

// columns are 0-indexed, we skip malformed lines
func parse2ColumnStats(fr fileReader, path string, keyColumn, valueColumn int, valueParser func([]byte, []byte) error) error {
	err := parseFile(fr, path, func(line []byte) error {

		var (
			i     int
			key   []byte
			value []byte
			token []byte
		)
		for len(line) != 0 {
			token, line = munchWhitespace(line)
			if i == keyColumn {
				key = token
			}
			if i == valueColumn {
				value = token
			}
			i++
		}
		return valueParser(key, value)
	})
	return err
}

// format is "some avg10=0.00 avg60=0.00 avg300=0.00 total=0"
func parsePSI(fr fileReader, path string, somePsi, fullPsi *PSIStats) error {
	return parseColumnStats(fr, path, func(fields []string) error {
		if len(fields) != 5 {
			reportError(newValueError("", fmt.Errorf("unexpected format for psi file at: %s, line content: %v", path, fields)))
			return nil
		}

		var psiStats *PSIStats

		switch fields[0] {
		case "some":
			psiStats = somePsi
		case "full":
			psiStats = fullPsi
		default:
			reportError(newValueError("", fmt.Errorf("unexpected psi type (some|full) for psi file at: %s, type: %s", path, fields[0])))
		}

		// User did not provide stat for this type or unknown PSI type
		if psiStats == nil {
			return nil
		}

		for i := 1; i < 5; i++ {
			parts := strings.Split(fields[i], "=")
			if len(parts) != 2 {
				reportError(newValueError("", fmt.Errorf("unexpected format for psi file at: %s, part: %d, content: %v", path, i, fields[i])))
				continue
			}

			psi, err := strconv.ParseFloat(parts[1], 64)
			if err != nil {
				reportError(newValueError("", fmt.Errorf("unexpected format for psi file at: %s, part: %d, content: %v", path, i, fields[i])))
				continue
			}

			switch parts[0] {
			case "avg10":
				psiStats.Avg10 = &psi
			case "avg60":
				psiStats.Avg60 = &psi
			case "avg300":
				psiStats.Avg300 = &psi
			case "total":
				total, err := strconv.ParseUint(parts[1], 10, 64)
				if err != nil {
					reportError(newValueError("", fmt.Errorf("unexpected format for psi file at: %s, part: %d, content: %v", path, i, fields[i])))
					continue
				}
				psiStats.Total = &total
			}
		}

		return nil
	})
}

// munchWhitespace reads the first token out of a unicode string, and returns a unicode string where the next call to munchWhitespace should pick up.
// `in` is expected to be a valid unicode string (represented as a []byte to allow for zero-copy). `token` and `rest` are slices aliasing the same memory
// as `in`.
//
// tok, rest := munchWhitespace("lorem ipsum dolor sit amet") => "lorem", "ipsum dolor sit amet"
// tok, rest := munchWhitespace("ipsum dolor sit amet", "dolor sit amet")
// ...
func munchWhitespace(in []byte) (token, rest []byte) {

	var cut int
	for cut < len(in) {
		r, size := utf8.DecodeRune(in[cut:])
		if unicode.IsSpace(r) {
			break
		}
		cut += size
	}

	cut2 := cut
	for cut2 < len(in) {
		r, size := utf8.DecodeRune(in[cut2:])
		if !unicode.IsSpace(r) {
			break
		}
		cut2 += size
	}
	return in[:cut], in[cut2:]
}
