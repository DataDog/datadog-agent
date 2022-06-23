// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestString(t *testing.T) {
	sketchSerie0 := SketchSeries{
		Name:     "sketchSerie0",
		Host:     "hostSketchSerie0",
		Interval: 10,
		Points: []SketchPoint{
			{Ts: 1},
			{Ts: 2},
		},
	}
	sketchSerie1 := SketchSeries{
		Name:     "sketchSerie1",
		Host:     "hostSketchSerie1",
		Interval: 100,
		Points: []SketchPoint{
			{Ts: 3},
			{Ts: 4},
		},
	}
	sketchSerieList := SketchSeriesList{
		&sketchSerie0,
		&sketchSerie1,
	}
	assert.Equal(t, "{\"sketches\":[{\"metric\":\"sketchSerie0\",\"tags\":[],\"host\":\"hostSketchSerie0\",\"interval\":10,\"points\":[{\"sketch\":null,\"ts\":1},{\"sketch\":null,\"ts\":2}]},{\"metric\":\"sketchSerie1\",\"tags\":[],\"host\":\"hostSketchSerie1\",\"interval\":100,\"points\":[{\"sketch\":null,\"ts\":3},{\"sketch\":null,\"ts\":4}]}]}\n", sketchSerieList.String())
	assert.Equal(
		t,
		sketchSerieList.String(),
		fmt.Sprintf("{\"sketches\":[%v,%v]}\n",
			strings.TrimSpace(sketchSerie0.String()),
			strings.TrimSpace(sketchSerie1.String())))
}
