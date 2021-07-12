// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"testing"
)

func TestStringBiMap(t *testing.T) {
	mymap := NewBiMap((string)(""), (string)(""))

	err := mymap.AddKV("a", "aa")
	if err != nil {
		t.Fatalf("Unable to insert key-value pair: %v", err)
	}

	err = mymap.AddKV("b", "bb")
	if err != nil {
		t.Fatalf("Unable to insert key-value pair: %v", err)
	}

	v, err := mymap.GetKV("a")
	if err != nil {
		t.Fatalf("Unable to get key-value pair: %v", err)
	}
	if v != "aa" {
		t.Fatalf("Unexpected value in key-value pair: %v", v)
	}

	v, err = mymap.GetKVReverse("bb")
	if err != nil {
		t.Fatalf("Unable to get key-value pair: %v", err)
	}
	if v != "b" {
		t.Fatalf("Unexpected value in key-value pair: %v", v)
	}
}

func TestStringIntBiMap(t *testing.T) {
	mymap := NewBiMap((string)(""), (int)(0))

	err := mymap.AddKV("a", 1)
	if err != nil {
		t.Fatalf("Unable to insert key-value pair: %v", err)
	}

	err = mymap.AddKV("b", 2)
	if err != nil {
		t.Fatalf("Unable to insert key-value pair: %v", err)
	}

	v, err := mymap.GetKV("a")
	if err != nil {
		t.Fatalf("Unable to get key-value pair: %v", err)
	}
	if v != 1 {
		t.Fatalf("Unexpected value in key-value pair: %v", v)
	}

	v, err = mymap.GetKVReverse(2)
	if err != nil {
		t.Fatalf("Unable to get key-value pair: %v", err)
	}
	if v != "b" {
		t.Fatalf("Unexpected value in key-value pair: %v", v)
	}
}
