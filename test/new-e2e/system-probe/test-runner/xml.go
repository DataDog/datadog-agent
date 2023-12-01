// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
)

// JUnitTestSuites is a collection of JUnit test suites.
type JUnitTestSuites struct {
	XMLName  xml.Name         `xml:"testsuites"`
	Name     string           `xml:"name,attr,omitempty"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Errors   int              `xml:"errors,attr"`
	Time     string           `xml:"time,attr"`
	Suites   []JUnitTestSuite `xml:"testsuite"`
}

// JUnitTestSuite is a single JUnit test suite which may contain many
// testcases.
type JUnitTestSuite struct {
	XMLName    xml.Name        `xml:"testsuite"`
	Tests      int             `xml:"tests,attr"`
	Failures   int             `xml:"failures,attr"`
	Time       string          `xml:"time,attr"`
	Name       string          `xml:"name,attr"`
	Properties []JUnitProperty `xml:"properties>property,omitempty"`
	TestCases  []JUnitTestCase `xml:"testcase"`
	Timestamp  string          `xml:"timestamp,attr"`
}

// JUnitTestCase is a single test case with its result.
type JUnitTestCase struct {
	XMLName     xml.Name          `xml:"testcase"`
	Classname   string            `xml:"classname,attr"`
	Name        string            `xml:"name,attr"`
	Time        string            `xml:"time,attr"`
	SkipMessage *JUnitSkipMessage `xml:"skipped,omitempty"`
	Failure     *JUnitFailure     `xml:"failure,omitempty"`
}

// JUnitSkipMessage contains the reason why a testcase was skipped.
type JUnitSkipMessage struct {
	Message string `xml:"message,attr"`
}

// JUnitProperty represents a key/value pair used to define properties.
type JUnitProperty struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

// JUnitFailure contains data related to a failed test.
type JUnitFailure struct {
	Message  string `xml:"message,attr"`
	Type     string `xml:"type,attr"`
	Contents string `xml:",chardata"`
}

func addProperties(xmlpath string, properties map[string]string) error {
	// create properties
	var props []JUnitProperty
	for k, v := range properties {
		props = append(props,
			JUnitProperty{
				Name:  k,
				Value: v,
			},
		)
	}

	// open and decode XML
	var suites JUnitTestSuites
	if err := openAndDecode(xmlpath, &suites); err != nil {
		return fmt.Errorf("xml decode: %w", err)
	}

	// add properties to XML
	for i := range suites.Suites {
		suites.Suites[i].Properties = append(suites.Suites[i].Properties, props...)
	}

	// write XML file
	f, err := os.OpenFile(xmlpath, os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer f.Close()
	doc, err := xml.MarshalIndent(suites, "", "\t")
	if err != nil {
		return fmt.Errorf("xml marshal: %w", err)
	}
	_, err = f.Write([]byte(xml.Header))
	if err != nil {
		return fmt.Errorf("xml write: %w", err)
	}
	_, err = f.Write(doc)
	return err
}

func openAndDecode(xmlpath string, suites *JUnitTestSuites) error {
	// open and decode XML
	f, err := os.OpenFile(xmlpath, os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	return decode(f, suites)
}

func decode(f io.Reader, suites *JUnitTestSuites) error {
	d := xml.NewDecoder(f)
	if err := d.Decode(suites); err != nil {
		return fmt.Errorf("xml decode: %w", err)
	}
	return nil
}
