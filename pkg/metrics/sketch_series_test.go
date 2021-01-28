package metrics

import (
	"bytes"
	"compress/zlib"
	"io/ioutil"
	"testing"

	"github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func check(t *testing.T, in SketchPoint, pb gogen.SketchPayload_Sketch_Dogsketch) {
	t.Helper()
	s, b := in.Sketch, in.Sketch.Basic
	require.Equal(t, in.Ts, pb.Ts)

	// sketch
	// k, n := s.Cols(make([]int32, s.Bins()), make([]uint32, s.Bins()))
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

func TestSketchSeriesListMarshal(t *testing.T) {
	sl := make(SketchSeriesList, 2)

	for i := range sl {
		sl[i] = Makeseries(i)
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
		require.Equal(t, Makeseries(i), in, "make sure we don't modify input")

		assert.Equal(t, in.Host, pb.Host)
		assert.Equal(t, in.Name, pb.Metric)
		assert.Equal(t, in.Tags, pb.Tags)
		assert.Len(t, pb.Distributions, 0)

		require.Len(t, pb.Dogsketches, len(in.Points))
		for j, pointPb := range pb.Dogsketches {

			check(t, in.Points[j], pointPb)
			// require.Equal(t, pointIn.Ts, pointPb.Ts)
			// require.Equal(t, pointIn.Ts, pointPb.Ts)

			// fmt.Printf("%#v %#v\n", pin, s)
		}
	}
}

func TestSketchSeriesListJSONMarshal(t *testing.T) {
	sl := make(SketchSeriesList, 2)

	for i := range sl {
		sl[i] = Makeseries(i)
	}

	json, err := sl.MarshalJSON()
	assert.NoError(t, err)
	assert.JSONEq(t, string(json), `{"sketches":[{"metric":"name.0","tags":["a:0","b:0"],"host":"host.0","interval":0,"points":[{"sketch":{"summary":{"Min":0,"Max":0,"Sum":0,"Avg":0,"Cnt":0}},"ts":0},{"sketch":{"summary":{"Min":0,"Max":0,"Sum":0,"Avg":0,"Cnt":1}},"ts":10},{"sketch":{"summary":{"Min":0,"Max":1,"Sum":1,"Avg":0.5,"Cnt":2}},"ts":20},{"sketch":{"summary":{"Min":0,"Max":2,"Sum":3,"Avg":1,"Cnt":3}},"ts":30},{"sketch":{"summary":{"Min":0,"Max":3,"Sum":6,"Avg":1.5,"Cnt":4}},"ts":40}]},{"metric":"name.1","tags":["a:1","b:1"],"host":"host.1","interval":1,"points":[{"sketch":{"summary":{"Min":0,"Max":0,"Sum":0,"Avg":0,"Cnt":0}},"ts":0},{"sketch":{"summary":{"Min":0,"Max":0,"Sum":0,"Avg":0,"Cnt":1}},"ts":10},{"sketch":{"summary":{"Min":0,"Max":1,"Sum":1,"Avg":0.5,"Cnt":2}},"ts":20},{"sketch":{"summary":{"Min":0,"Max":2,"Sum":3,"Avg":1,"Cnt":3}},"ts":30},{"sketch":{"summary":{"Min":0,"Max":3,"Sum":6,"Avg":1.5,"Cnt":4}},"ts":40},{"sketch":{"summary":{"Min":0,"Max":4,"Sum":10,"Avg":2,"Cnt":5}},"ts":50}]}]}`)

	config.Datadog.Set("cmd.check.fullsketches", true)
	json, err = sl.MarshalJSON()
	assert.NoError(t, err)
	assert.JSONEq(t, string(json), `{"sketches":[{"host":"host.0","interval":0,"metric":"name.0","points":[{"bins":"","binsCount":0,"sketch":{"summary":{"Avg":0,"Cnt":0,"Max":0,"Min":0,"Sum":0}},"ts":0},{"bins":"0:1","binsCount":1,"sketch":{"summary":{"Avg":0,"Cnt":1,"Max":0,"Min":0,"Sum":0}},"ts":10},{"bins":"0:1 1338:1","binsCount":2,"sketch":{"summary":{"Avg":0.5,"Cnt":2,"Max":1,"Min":0,"Sum":1}},"ts":20},{"bins":"0:1 1338:1 1383:1","binsCount":3,"sketch":{"summary":{"Avg":1,"Cnt":3,"Max":2,"Min":0,"Sum":3}},"ts":30},{"bins":"0:1 1338:1 1383:1 1409:1","binsCount":4,"sketch":{"summary":{"Avg":1.5,"Cnt":4,"Max":3,"Min":0,"Sum":6}},"ts":40}],"tags":["a:0","b:0"]},{"host":"host.1","interval":1,"metric":"name.1","points":[{"bins":"","binsCount":0,"sketch":{"summary":{"Avg":0,"Cnt":0,"Max":0,"Min":0,"Sum":0}},"ts":0},{"bins":"0:1","binsCount":1,"sketch":{"summary":{"Avg":0,"Cnt":1,"Max":0,"Min":0,"Sum":0}},"ts":10},{"bins":"0:1 1338:1","binsCount":2,"sketch":{"summary":{"Avg":0.5,"Cnt":2,"Max":1,"Min":0,"Sum":1}},"ts":20},{"bins":"0:1 1338:1 1383:1","binsCount":3,"sketch":{"summary":{"Avg":1,"Cnt":3,"Max":2,"Min":0,"Sum":3}},"ts":30},{"bins":"0:1 1338:1 1383:1 1409:1","binsCount":4,"sketch":{"summary":{"Avg":1.5,"Cnt":4,"Max":3,"Min":0,"Sum":6}},"ts":40},{"bins":"0:1 1338:1 1383:1 1409:1 1427:1","binsCount":5,"sketch":{"summary":{"Avg":2,"Cnt":5,"Max":4,"Min":0,"Sum":10}},"ts":50}],"tags":["a:1","b:1"]}]}`)
}

func TestSketchSeriesStreamCompressPayloads(t *testing.T) {
	sl := make(SketchSeriesList, 2)

	for i := range sl {
		sl[i] = Makeseries(i)
	}

	payload, _ := sl.Marshal() // old way
	// payloads, noncompressed := sl.StreamCompressPayloads() // new compressed
	payloads, err := sl.StreamCompressPayloads() // new compressed

	assert.Nil(t, err)

	reader := bytes.NewReader(*payloads[0])
	r, e := zlib.NewReader(reader)
	decompressed, ee := ioutil.ReadAll(r)
	r.Close()

	_ = e
	_ = ee
	_ = payload
	_ = decompressed
	// _ = noncompressed

	// Check that we encoded the protobuf correctly
	assert.Equal(t, decompressed, payload)

	pl := new(gogen.SketchPayload)
	if err := pl.Unmarshal(decompressed); err != nil {
		t.Fatal(err)
	}

	require.Len(t, pl.Sketches, len(sl))

	for i, pb := range pl.Sketches {
		in := sl[i]
		require.Equal(t, Makeseries(i), in, "make sure we don't modify input")

		assert.Equal(t, in.Host, pb.Host)
		assert.Equal(t, in.Name, pb.Metric)
		assert.Equal(t, in.Tags, pb.Tags)
		assert.Len(t, pb.Distributions, 0)

		require.Len(t, pb.Dogsketches, len(in.Points))
		for j, pointPb := range pb.Dogsketches {

			check(t, in.Points[j], pointPb)
		}
	}
}

func TestSketchSeriesStreamCompressPayloadsSplit(t *testing.T) {
	sl := make(SketchSeriesList, 200)

	for i := range sl {
		sl[i] = Makeseries(i)
	}

	payloads, err := sl.StreamCompressPayloads()
	assert.Nil(t, err)

	recoveredSketches := []gogen.SketchPayload{}
	recoveredCount := 0
	for _, pld := range payloads {
		reader := bytes.NewReader(*pld)
		r, _ := zlib.NewReader(reader)
		decompressed, _ := ioutil.ReadAll(r)
		r.Close()

		pl := new(gogen.SketchPayload)
		if err := pl.Unmarshal(decompressed); err != nil {
			t.Fatal(err)
		}
		recoveredSketches = append(recoveredSketches, *pl)
		recoveredCount += len(pl.Sketches)
	}

	assert.Equal(t, recoveredCount, len(sl))

	i := 0
	for _, pl := range recoveredSketches {
		for _, pb := range pl.Sketches {
			in := sl[i]
			require.Equal(t, Makeseries(i), in, "make sure we don't modify input")

			assert.Equal(t, in.Host, pb.Host)
			assert.Equal(t, in.Name, pb.Metric)
			assert.Equal(t, in.Tags, pb.Tags)
			assert.Len(t, pb.Distributions, 0)

			require.Len(t, pb.Dogsketches, len(in.Points))
			for j, pointPb := range pb.Dogsketches {

				check(t, in.Points[j], pointPb)
			}
			i++
		}
	}
}
