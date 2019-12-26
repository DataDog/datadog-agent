package metrics

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/quantile"

	"github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSketchSeriesListMarshal(t *testing.T) {
	var (
		sl         = make(SketchSeriesList, 2)
		makesketch = func(n int) *quantile.Sketch {
			s, c := &quantile.Sketch{}, quantile.Default()
			for i := 0; i < n; i++ {
				s.Insert(c, float64(i))
			}
			return s
		}

		// makeseries is deterministic so that we can test for mutation.
		makeseries = func(i int) SketchSeries {
			ss := SketchSeries{
				Name: fmt.Sprintf("name.%d", i),
				Tags: []string{
					fmt.Sprintf("a:%d", i),
					fmt.Sprintf("b:%d", i),
				},
				Host:     fmt.Sprintf("host.%d", i),
				Interval: int64(i),
			}

			for j := 0; j < i+5; j++ {
				ss.Points = append(ss.Points, SketchPoint{
					Ts:     10 * int64(j),
					Sketch: makesketch(j),
				})
			}

			gen := ckey.NewKeyGenerator()
			ss.ContextKey = gen.Generate(ss.Name, ss.Host, ss.Tags)

			return ss
		}

		check = func(in SketchPoint, pb gogen.SketchPayload_Sketch_Dogsketch) {
			s, b := in.Sketch, in.Sketch.Basic
			require.Equal(t, in.Ts, pb.Ts)

			// sketch
			k, n := s.Cols()
			require.Equal(t, k, pb.K)
			require.Equal(t, n, pb.N)

			// summary
			require.Equal(t, b.Cnt, pb.Cnt)
			require.Equal(t, b.Min, pb.Min)
			require.Equal(t, b.Max, pb.Max)
			require.Equal(t, b.Avg, pb.Avg)
			require.Equal(t, b.Sum, pb.Sum)
		}
	)

	for i := range sl {
		sl[i] = makeseries(i)
	}

	b, err := sl.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	pl := new(gogen.SketchPayload)
	if err := pl.Unmarshal(b); err != nil {
		t.Fatal(err)
	}

	require.Len(t, pl.Sketches, len(sl))

	for i, pb := range pl.Sketches {
		in := sl[i]
		require.Equal(t, makeseries(i), in, "make sure we don't modify input")

		assert.Equal(t, in.Host, pb.Host)
		assert.Equal(t, in.Name, pb.Metric)
		assert.Equal(t, in.Tags, pb.Tags)
		assert.Len(t, pb.Distributions, 0)

		require.Len(t, pb.Dogsketches, len(in.Points))
		for j, pointPb := range pb.Dogsketches {

			check(in.Points[j], pointPb)
			// require.Equal(t, pointIn.Ts, pointPb.Ts)
			// require.Equal(t, pointIn.Ts, pointPb.Ts)

			// fmt.Printf("%#v %#v\n", pin, s)
		}
	}

}
