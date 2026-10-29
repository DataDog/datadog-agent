// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ast holds the abstract syntax tree of the SECL language.
package ast

import (
	"fmt"
	"strconv"
)

// Kind is the kind of a Value.
type Kind int

// Kinds of values. The zero value is not a valid kind.
const (
	// Bool is a boolean: `true`.
	Bool Kind = iota + 1
	// Int is an integer: `123`.
	Int
	// Duration is a duration, in nanoseconds: `10s`.
	Duration
	// String is a string: `"/etc/passwd"`. It may hold `${variable}` and
	// `%{field}` interpolations.
	String
	// Pattern is a glob pattern: `~"/etc/*"`.
	Pattern
	// Regexp is a regular expression: `r"/etc/.*"`.
	Regexp
	// IP is an IP: `127.0.0.1`.
	IP
	// CIDR is a CIDR: `192.168.0.0/24`.
	CIDR
)

func (k Kind) String() string {
	switch k {
	case Bool:
		return "bool"
	case Int:
		return "int"
	case Duration:
		return "duration"
	case String:
		return "string"
	case Pattern:
		return "pattern"
	case Regexp:
		return "regexp"
	case IP:
		return "IP"
	case CIDR:
		return "CIDR"
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// Value is a value of the language, the value of a Literal.
//
// The union is closed, the only implementations are the ones of this package,
// built by the Make functions. The value of a given kind is read with the
// accessor of the kind: BoolVal, IntVal, StringVal.
type Value interface {
	// Kind returns the kind of the value.
	Kind() Kind
	// String returns the value as written in an expression: `10ns`, `~"/etc/*"`.
	String() string

	// value closes the union.
	value()
}

type (
	boolVal     bool
	intVal      int
	durationVal int
	stringVal   string
	patternVal  string
	regexpVal   string
	ipVal       string
	cidrVal     string
)

// MakeBool returns the Bool value of a boolean.
func MakeBool(value bool) Value { return boolVal(value) }

// MakeInt returns the Int value of an integer.
func MakeInt(value int) Value { return intVal(value) }

// MakeDuration returns the Duration value of a number of nanoseconds.
func MakeDuration(nanoseconds int) Value { return durationVal(nanoseconds) }

// MakeString returns the String value of a string, unquoted.
func MakeString(value string) Value { return stringVal(value) }

// MakePattern returns the Pattern value of a glob pattern, unquoted.
func MakePattern(value string) Value { return patternVal(value) }

// MakeRegexp returns the Regexp value of a regular expression, unquoted.
func MakeRegexp(value string) Value { return regexpVal(value) }

// MakeIP returns the IP value of the textual representation of an IP.
func MakeIP(value string) Value { return ipVal(value) }

// MakeCIDR returns the CIDR value of the textual representation of a CIDR.
func MakeCIDR(value string) Value { return cidrVal(value) }

// BoolVal returns the boolean of a Bool value. It panics for any other kind.
func BoolVal(v Value) bool {
	if v, ok := v.(boolVal); ok {
		return bool(v)
	}
	panic(fmt.Sprintf("BoolVal of a %s value", v.Kind()))
}

// IntVal returns the integer of an Int value, the nanoseconds of a Duration
// value. It panics for any other kind.
func IntVal(v Value) int {
	switch v := v.(type) {
	case intVal:
		return int(v)
	case durationVal:
		return int(v)
	}
	panic(fmt.Sprintf("IntVal of a %s value", v.Kind()))
}

// StringVal returns the string of a String, Pattern or Regexp value, unquoted,
// the textual representation of an IP or CIDR value. It panics for any other
// kind.
func StringVal(v Value) string {
	switch v := v.(type) {
	case stringVal:
		return string(v)
	case patternVal:
		return string(v)
	case regexpVal:
		return string(v)
	case ipVal:
		return string(v)
	case cidrVal:
		return string(v)
	}
	panic(fmt.Sprintf("StringVal of a %s value", v.Kind()))
}

func (boolVal) Kind() Kind     { return Bool }
func (intVal) Kind() Kind      { return Int }
func (durationVal) Kind() Kind { return Duration }
func (stringVal) Kind() Kind   { return String }
func (patternVal) Kind() Kind  { return Pattern }
func (regexpVal) Kind() Kind   { return Regexp }
func (ipVal) Kind() Kind       { return IP }
func (cidrVal) Kind() Kind     { return CIDR }

func (v boolVal) String() string     { return strconv.FormatBool(bool(v)) }
func (v intVal) String() string      { return strconv.Itoa(int(v)) }
func (v durationVal) String() string { return strconv.Itoa(int(v)) + "ns" }
func (v stringVal) String() string   { return strconv.Quote(string(v)) }
func (v patternVal) String() string  { return "~" + strconv.Quote(string(v)) }
func (v regexpVal) String() string   { return "r" + strconv.Quote(string(v)) }
func (v ipVal) String() string       { return string(v) }
func (v cidrVal) String() string     { return string(v) }

func (boolVal) value()     {}
func (intVal) value()      {}
func (durationVal) value() {}
func (stringVal) value()   {}
func (patternVal) value()  {}
func (regexpVal) value()   {}
func (ipVal) value()       {}
func (cidrVal) value()     {}
