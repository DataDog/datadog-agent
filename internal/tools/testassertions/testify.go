// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"strconv"
	"strings"
)

const (
	assertPkg  = "github.com/stretchr/testify/assert"
	requirePkg = "github.com/stretchr/testify/require"
	suitePkg   = "github.com/stretchr/testify/suite"
	e2ePkg     = repoModule + "/test/e2e-framework/testing/e2e"
)

// assertSpec describes a testify assertion: how many arguments are meaningful
// (the rest is msgAndArgs) and a human readable template where {i} is the
// i-th meaningful argument (after the TestingT one).
type assertSpec struct {
	arity int
	tmpl  string
}

var testifySpecs = map[string]assertSpec{
	"Equal":               {2, "{1} == {0}"},
	"EqualValues":         {2, "{1} == {0} (after conversion)"},
	"EqualExportedValues": {2, "exported fields of {1} == {0}"},
	"Exactly":             {2, "{1} == {0} (same type)"},
	"NotEqual":            {2, "{1} != {0}"},
	"NotEqualValues":      {2, "{1} != {0} (after conversion)"},
	"Same":                {2, "{1} is the same pointer as {0}"},
	"NotSame":             {2, "{1} is not the same pointer as {0}"},
	"True":                {1, "{0}"},
	"False":               {1, "!({0})"},
	"Nil":                 {1, "{0} is nil"},
	"NotNil":              {1, "{0} is not nil"},
	"Empty":               {1, "{0} is empty"},
	"NotEmpty":            {1, "{0} is not empty"},
	"Zero":                {1, "{0} is the zero value"},
	"NotZero":             {1, "{0} is not the zero value"},
	"Len":                 {2, "len({0}) == {1}"},
	"Contains":            {2, "{0} contains {1}"},
	"NotContains":         {2, "{0} does not contain {1}"},
	"Subset":              {2, "{0} contains all of {1}"},
	"NotSubset":           {2, "{0} does not contain all of {1}"},
	"ElementsMatch":       {2, "{0} has the same elements as {1} (any order)"},
	"NotElementsMatch":    {2, "{0} does not have the same elements as {1}"},
	"NoError":             {1, "{0} is nil (no error)"},
	"Error":               {1, "{0} is an error"},
	"EqualError":          {2, "{0}.Error() == {1}"},
	"ErrorContains":       {2, "{0}.Error() contains {1}"},
	"ErrorIs":             {2, "errors.Is({0}, {1})"},
	"NotErrorIs":          {2, "!errors.Is({0}, {1})"},
	"ErrorAs":             {2, "errors.As({0}, {1})"},
	"NotErrorAs":          {2, "!errors.As({0}, {1})"},
	"Greater":             {2, "{0} > {1}"},
	"GreaterOrEqual":      {2, "{0} >= {1}"},
	"Less":                {2, "{0} < {1}"},
	"LessOrEqual":         {2, "{0} <= {1}"},
	"Positive":            {1, "{0} > 0"},
	"Negative":            {1, "{0} < 0"},
	"IsIncreasing":        {1, "{0} is strictly increasing"},
	"IsDecreasing":        {1, "{0} is strictly decreasing"},
	"IsNonIncreasing":     {1, "{0} is non-increasing"},
	"IsNonDecreasing":     {1, "{0} is non-decreasing"},
	"InDelta":             {3, "|{0} - {1}| <= {2}"},
	"InDeltaSlice":        {3, "|{0}[i] - {1}[i]| <= {2}"},
	"InDeltaMapValues":    {3, "|{0}[k] - {1}[k]| <= {2}"},
	"InEpsilon":           {3, "relative error between {0} and {1} <= {2}"},
	"InEpsilonSlice":      {3, "relative error between {0}[i] and {1}[i] <= {2}"},
	"WithinDuration":      {3, "|{0} - {1}| <= {2}"},
	"WithinRange":         {3, "{1} <= {0} <= {2}"},
	"Regexp":              {2, "{1} matches regexp {0}"},
	"NotRegexp":           {2, "{1} does not match regexp {0}"},
	"JSONEq":              {2, "JSON {1} is equivalent to {0}"},
	"YAMLEq":              {2, "YAML {1} is equivalent to {0}"},
	"FileExists":          {1, "file {0} exists"},
	"NoFileExists":        {1, "file {0} does not exist"},
	"DirExists":           {1, "directory {0} exists"},
	"NoDirExists":         {1, "directory {0} does not exist"},
	"FileEmpty":           {1, "file {0} is empty"},
	"FileNotEmpty":        {1, "file {0} is not empty"},
	"IsType":              {2, "{1} has the type of {0}"},
	"IsNotType":           {2, "{1} does not have the type of {0}"},
	"Implements":          {2, "{1} implements {0}"},
	"NotImplements":       {2, "{1} does not implement {0}"},
	"Panics":              {1, "{0} panics"},
	"NotPanics":           {1, "{0} does not panic"},
	"PanicsWithValue":     {2, "{1} panics with value {0}"},
	"PanicsWithError":     {2, "{1} panics with error {0}"},
	"Condition":           {1, "{0} returns true"},
	"Eventually":          {3, ""},
	"EventuallyWithT":     {3, ""},
	// e2e.BaseSuite: polls a func() error with exponential backoff until it returns nil
	"EventuallyWithExponentialBackoff": {3, ""},
	"Never":                            {3, ""},
	"Fail":                             {1, "fails: {0}"},
	"FailNow":                          {1, "fails: {0}"},
	"HTTPSuccess":                      {4, "HTTP {1} {2} returns a 2xx status"},
	"HTTPError":                        {4, "HTTP {1} {2} returns an error status"},
	"HTTPRedirect":                     {4, "HTTP {1} {2} returns a redirect"},
	"HTTPStatusCode":                   {5, "HTTP {1} {2} returns status {4}"},
	"HTTPBodyContains":                 {5, "HTTP {1} {2} body contains {4}"},
	"HTTPBodyNotContains":              {5, "HTTP {1} {2} body does not contain {4}"},
}

// polling assertions take a function that is evaluated repeatedly.
var pollingAssertions = map[string]bool{"Eventually": true, "EventuallyWithT": true, "Never": true, "Condition": true, "EventuallyWithExponentialBackoff": true}

// lookupSpec normalizes "f" variants (Equalf -> Equal).
func lookupSpec(name string) (base string, spec assertSpec, formatted, ok bool) {
	if s, found := testifySpecs[name]; found {
		return name, s, false, true
	}
	if trimmed := strings.TrimSuffix(name, "f"); trimmed != name {
		if s, found := testifySpecs[trimmed]; found {
			return trimmed, s, true, true
		}
	}
	return name, assertSpec{}, false, false
}

func fillTemplate(tmpl string, args []string) string {
	out := tmpl
	for i := 0; i < 6; i++ {
		v := "?"
		if i < len(args) {
			v = args[i]
		}
		out = strings.ReplaceAll(out, "{"+strconv.Itoa(i)+"}", v)
	}
	return out
}

// testingFailures are the *testing.T / CollectT methods that fail a test.
var testingFailures = map[string]bool{"Error": true, "Errorf": true, "Fatal": true, "Fatalf": true, "Fail": true, "FailNow": true}

// lifecycleHooks are testify suite hooks that may contain assertions.
var lifecycleHooks = map[string]bool{
	"SetupSuite": true, "SetupTest": true, "BeforeTest": true, "SetupSubTest": true,
	"AfterTest": true, "TearDownTest": true, "TearDownSubTest": true, "TearDownSuite": true,
}
