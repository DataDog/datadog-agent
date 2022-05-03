// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	_ "github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/stretchr/testify/require"
)

func TestIterableSeries(t *testing.T) {
	iterableSeries := IterableSeries{IterableSeries: metrics.NewIterableSeries(func(*metrics.Serie) {}, 10, 1)}
	done := make(chan struct{})
	var descritions []string
	go func() {
		defer iterableSeries.IterationStopped()
		for iterableSeries.MoveNext() {
			descritions = append(descritions, iterableSeries.DescribeCurrentItem())
		}
		close(done)
	}()
	iterableSeries.Append(&metrics.Serie{Name: "serie1"})
	iterableSeries.Append(&metrics.Serie{Name: "serie2"})
	iterableSeries.Append(&metrics.Serie{Name: "serie3"})
	iterableSeries.SenderStopped()
	<-done
	r := require.New(t)
	r.Len(descritions, 3)
	r.True(strings.Contains(descritions[0], "serie1"))
	r.True(strings.Contains(descritions[1], "serie2"))
	r.True(strings.Contains(descritions[2], "serie3"))
}

func TestIterableSeriesCallback(t *testing.T) {
	var series Series
	callback := func(s *metrics.Serie) { series = append(series, s) }
	iterableSeries := IterableSeries{IterableSeries: metrics.NewIterableSeries(callback, 10, 10)}
	iterableSeries.Append(&metrics.Serie{Name: "serie1"})
	iterableSeries.Append(&metrics.Serie{Name: "serie2"})

	r := require.New(t)
	r.Equal(uint64(2), iterableSeries.SeriesCount())
	r.Len(series, 2)
	r.Equal("serie1", series[0].Name)
	r.Equal("serie2", series[1].Name)
}

func TestIterableSeriesReceiverStopped(t *testing.T) {
	iterableSeries := IterableSeries{IterableSeries: metrics.NewIterableSeries(func(*metrics.Serie) {}, 1, 1)}
	iterableSeries.Append(&metrics.Serie{Name: "serie1"})

	// Next call to Append must not block
	go iterableSeries.IterationStopped()
	iterableSeries.Append(&metrics.Serie{Name: "serie2"})
	iterableSeries.Append(&metrics.Serie{Name: "serie3"})
}

func BenchmarkIterableSeries(b *testing.B) {
	for bufferSize := 1000; bufferSize <= 8000; bufferSize *= 2 {
		b.Run(fmt.Sprintf("%v", bufferSize), func(b *testing.B) {
			iterableSeries := IterableSeries{IterableSeries: metrics.NewIterableSeries(func(*metrics.Serie) {}, 100, bufferSize)}
			done := make(chan struct{})
			go func() {
				defer iterableSeries.IterationStopped()
				for iterableSeries.MoveNext() {
				}
				close(done)
			}()

			for i := 0; i < b.N; i++ {
				iterableSeries.Append(&metrics.Serie{Name: "name"})
			}
			iterableSeries.SenderStopped()
			<-done
		})
	}
}

func TestIterableSeriesSeveralValues(t *testing.T) {
	iterableSeries := IterableSeries{IterableSeries: metrics.NewIterableSeries(func(*metrics.Serie) {}, 10, 2)}
	done := make(chan struct{})
	var series []*metrics.Serie
	go func() {
		defer iterableSeries.IterationStopped()
		for iterableSeries.MoveNext() {
			series = append(series, iterableSeries.Current())
		}
		close(done)
	}()
	var expected []string
	for i := 0; i < 101; i++ {
		name := "serie" + strconv.Itoa(i)
		expected = append(expected, name)
		iterableSeries.Append(&metrics.Serie{Name: name})
	}
	iterableSeries.SenderStopped()
	<-done
	r := require.New(t)
	r.Len(series, len(expected))
	for i, v := range expected {
		r.Equal(v, series[i].Name)
	}
}

func TestIterableSeriesEmptyMarshalJSON(t *testing.T) {
	r := require.New(t)
	iterableSeries := IterableSeries{IterableSeries: metrics.NewIterableSeries(func(*metrics.Serie) {}, 10, 2)}
	iterableSeries.SenderStopped()
	bytes, err := iterableSeries.MarshalJSON()
	r.NoError(err)
	r.Equal(`{"series":[]}`, strings.TrimSpace(string(bytes)))
}
