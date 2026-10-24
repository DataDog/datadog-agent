// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package jq

import (
	"errors"
	"io"
)

var (
	errInvalidSurrogate = errors.New("invalid Unicode surrogate escape")
)

type surrogateValidator struct {
	reader io.Reader

	inString    bool
	escaped     bool
	digitsLeft  int
	codeUnit    uint16
	lowUnit     bool
	pendingHigh bool
	needLowU    bool
	invalid     bool
	terminalErr error
}

func (v *surrogateValidator) scanByte(c byte) error {
	if !v.inString {
		if c == '"' {
			v.resetString()
			v.inString = true
		}
		return nil
	}
	closes := c == '"' && !v.escaped
	if !v.invalid && v.invalidSurrogateByte(c) {
		v.invalid = true
	}
	if v.escaped {
		v.escaped = false
	} else {
		switch c {
		case '\\':
			v.escaped = true
		case '"':
			v.inString = false
		}
	}
	if !closes {
		return nil
	}
	if v.invalid {
		return errInvalidSurrogate
	}
	v.resetString()
	return nil
}

func (v *surrogateValidator) invalidSurrogateByte(c byte) bool {
	if v.digitsLeft > 0 {
		digit, ok := hexDigit(c)
		if !ok {
			// encoding/json will report the malformed hexadecimal escape.
			v.digitsLeft = 0
			v.lowUnit = false
			return false
		}
		v.codeUnit = v.codeUnit<<4 | digit
		v.digitsLeft--
		if v.digitsLeft == 0 {
			if v.lowUnit {
				if v.codeUnit < 0xdc00 || v.codeUnit > 0xdfff {
					return true
				}
				v.lowUnit = false
			} else if v.codeUnit >= 0xd800 && v.codeUnit <= 0xdbff {
				v.pendingHigh = true
			}
			// jq decodes unpaired low surrogates to U+FFFD.
		}
		return false
	}
	if v.needLowU {
		if c != 'u' {
			return true
		}
		v.needLowU = false
		v.lowUnit = true
		v.digitsLeft = 4
		v.codeUnit = 0
		return false
	}
	if v.pendingHigh {
		if c != '\\' {
			return true
		}
		v.pendingHigh = false
		v.needLowU = true
		return false
	}
	if v.escaped {
		if c == 'u' {
			v.digitsLeft = 4
			v.codeUnit = 0
		}
		return false
	}
	return false
}

func (v *surrogateValidator) resetString() {
	v.escaped = false
	v.digitsLeft = 0
	v.codeUnit = 0
	v.lowUnit = false
	v.pendingHigh = false
	v.needLowU = false
	v.invalid = false
}

func (v *surrogateValidator) scan(data []byte) (int, error) {
	for i, c := range data {
		if err := v.scanByte(c); err != nil {
			return i + 1, err
		}
	}
	return len(data), nil
}

func (v *surrogateValidator) Read(p []byte) (int, error) {
	if v.terminalErr != nil {
		return 0, v.terminalErr
	}
	n, err := v.reader.Read(p)
	consumed, scanErr := v.scan(p[:n])
	if scanErr != nil {
		v.terminalErr = scanErr
		return consumed, nil
	}
	return n, err
}

func validateSurrogates(text string) error {
	v := &surrogateValidator{}
	for i := 0; i < len(text); i++ {
		if err := v.scanByte(text[i]); err != nil {
			return err
		}
	}
	return v.finish()
}

func validateSurrogatesBytes(text []byte) error {
	v := &surrogateValidator{}
	for _, c := range text {
		if err := v.scanByte(c); err != nil {
			return err
		}
	}
	return v.finish()
}

func (v *surrogateValidator) finish() error {
	if v.invalid || v.pendingHigh || v.needLowU || v.lowUnit {
		return errInvalidSurrogate
	}
	return nil
}

func hexDigit(c byte) (uint16, bool) {
	switch {
	case c >= '0' && c <= '9':
		return uint16(c - '0'), true
	case c >= 'a' && c <= 'f':
		return uint16(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return uint16(c-'A') + 10, true
	default:
		return 0, false
	}
}
