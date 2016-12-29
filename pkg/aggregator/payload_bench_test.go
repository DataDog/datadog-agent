package aggregator

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pb"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"gopkg.in/vmihailenco/msgpack.v2"
	"math/rand"
	"testing"
	"time"
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(max int) string {
	n := rand.Intn(max)
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)

}

func randTS() int64 {
	offset := int64(rand.Intn(3600))
	return time.Now().Unix() - offset

}

func randValue() float64 {

	return float64(rand.Intn(100000)) * rand.Float64()

}

func makePayload(nbSeries int, ptsPerSerie int, tagsPerSerie int) *pb.MetricsPayload {

	ts := make([]*pb.MetricsPayload_Timeserie, nbSeries)

	for i := 0; i < nbSeries; i++ {

		points := make([]*pb.MetricsPayload_Timeserie_Point, ptsPerSerie)

		for j := 0; j < ptsPerSerie; j++ {

			pt := pb.MetricsPayload_Timeserie_Point{
				Ts:    randTS(),
				Value: randValue(),
			}

			points[j] = &pt
		}

		tags := make([]string, tagsPerSerie)
		for j := 0; j < tagsPerSerie; j++ {
			tags[j] = randSeq(30)

		}

		s := pb.MetricsPayload_Timeserie{
			Metric:   fmt.Sprintf("dd.%s.%s", randSeq(10), randSeq(10)),
			Points:   points,
			Tags:     tags,
			Host:     randSeq(12),
			Type:     "gauge",
			Interval: 10,
		}

		ts[i] = &s

	}

	return &pb.MetricsPayload{
		Timeseries: ts,
	}

}

func benchmarkProto(nbSeries int, ptsPerSerie int, tagsPerSerie int, b *testing.B) {
	payload := makePayload(nbSeries, ptsPerSerie, tagsPerSerie)

	data, _ := proto.Marshal(payload)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {

		unmarshalledPayload := &pb.MetricsPayload{}
		proto.Unmarshal(data, unmarshalledPayload)

	}

}

func benchmarkProtoGoFast(nbSeries int, ptsPerSerie int, tagsPerSerie int, b *testing.B) {
	payload := makePayload(nbSeries, ptsPerSerie, tagsPerSerie)

	data, _ := proto.Marshal(payload)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {

		unmarshalledPayload := &pb.MetricsPayloadGoFast{}
		proto.Unmarshal(data, unmarshalledPayload)

	}

}

type MetricsPayload struct {
	Timeseries []*MetricsPayload_Timeserie `json:"timeseries"`
}

type MetricsPayload_Timeserie struct {
	Metric   string                            `json:"metric"`
	Type     string                            `json:"type"`
	Host     string                            `json:"host"`
	Points   []*MetricsPayload_Timeserie_Point `json:"points"`
	Tags     []string                          `json:"tags"`
	Interval int32                             `json:"interval"`
}

type MetricsPayload_Timeserie_Point struct {
	Ts    int64   `json:"ts"`
	Value float64 `json:"value"`
}

func benchmarkJSON(nbSeries int, ptsPerSerie int, tagsPerSerie int, b *testing.B) {
	payload := makePayload(nbSeries, ptsPerSerie, tagsPerSerie)

	marshaller := jsonpb.Marshaler{
		EnumsAsInts:  true,
		EmitDefaults: false,
		OrigName:     true,
	}

	var buff bytes.Buffer
	writer := bufio.NewWriter(&buff)

	marshaller.Marshal(writer, payload)
	writer.Flush()
	buby := buff.Bytes()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var data MetricsPayload
		json.Unmarshal(buby, &data)
	}

}

func benchmarkMSGPack(nbSeries int, ptsPerSerie int, tagsPerSerie int, b *testing.B) {
	payload := makePayload(nbSeries, ptsPerSerie, tagsPerSerie)

	marshalled, _ := msgpack.Marshal(&payload)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var data MetricsPayload
		msgpack.Unmarshal(marshalled, &data)
	}
}

func BenchmarkProto1_1_1(b *testing.B)       { benchmarkProto(1, 1, 1, b) }
func BenchmarkProtoGoFast1_1_1(b *testing.B) { benchmarkProtoGoFast(1, 1, 1, b) }
func BenchmarkJSON1_1_1(b *testing.B)        { benchmarkJSON(1, 1, 1, b) }
func BenchmarkMSGPack1_1_1(b *testing.B)     { benchmarkMSGPack(1, 1, 1, b) }

func BenchmarkProto10_10_10(b *testing.B)       { benchmarkProto(10, 10, 10, b) }
func BenchmarkProtoGoFast10_10_10(b *testing.B) { benchmarkProtoGoFast(10, 10, 10, b) }
func BenchmarkJSON10_10_10(b *testing.B)        { benchmarkJSON(10, 10, 10, b) }
func BenchmarkMSGPack10_10_10(b *testing.B)     { benchmarkMSGPack(10, 10, 10, b) }

func BenchmarkProto100_100_100(b *testing.B)       { benchmarkProto(100, 100, 100, b) }
func BenchmarkProtoGoFast100_100_100(b *testing.B) { benchmarkProtoGoFast(100, 100, 100, b) }
func BenchmarkJSON100_100_100(b *testing.B)        { benchmarkJSON(100, 100, 100, b) }
func BenchmarkMSGPack100_100_100(b *testing.B)     { benchmarkMSGPack(100, 100, 100, b) }

func BenchmarkProto1000_1000_1000(b *testing.B)       { benchmarkProto(1000, 1000, 1000, b) }
func BenchmarkProtoGoFast1000_1000_1000(b *testing.B) { benchmarkProtoGoFast(1000, 1000, 1000, b) }
func BenchmarkJSON1000_1000_1000(b *testing.B)        { benchmarkJSON(1000, 1000, 1000, b) }
func BenchmarkMSGPack1000_1000_1000(b *testing.B)     { benchmarkMSGPack(1000, 1000, 1000, b) }

func BenchmarkProto1_1_1000(b *testing.B)       { benchmarkProto(1, 1, 1000, b) }
func BenchmarkProtoGoFast1_1_1000(b *testing.B) { benchmarkProtoGoFast(1, 1, 1000, b) }
func BenchmarkJSON1_1_1000(b *testing.B)        { benchmarkJSON(1, 1, 1000, b) }
func BenchmarkMSGPack1_1_1000(b *testing.B)     { benchmarkMSGPack(1, 1, 1000, b) }

func BenchmarkProto1_1000_1(b *testing.B)       { benchmarkProto(1, 1000, 1, b) }
func BenchmarkProtoGoFast1_1000_1(b *testing.B) { benchmarkProtoGoFast(1, 1000, 1, b) }
func BenchmarkJSON1_1000_1(b *testing.B)        { benchmarkJSON(1, 1000, 1, b) }
func BenchmarkMSGPack1_1000_1(b *testing.B)     { benchmarkMSGPack(1, 1000, 1, b) }

func BenchmarkProto1000_1_0(b *testing.B)       { benchmarkProtoGoFast(1000, 1, 1, b) }
func BenchmarkProtoGoFast1000_1_0(b *testing.B) { benchmarkProtoGoFast(1000, 1, 1, b) }
func BenchmarkJSON1000_1_0(b *testing.B)        { benchmarkJSON(1000, 1, 1, b) }
func BenchmarkMSGPack1000_1_1(b *testing.B)     { benchmarkMSGPack(1000, 1, 1, b) }
