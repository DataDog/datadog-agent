// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package tpu

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// vbarAddress is the TPU VBAR control service of a TPU VM, which reads chip
	// activity on the host without a workload.
	vbarAddress = "localhost:8353"
	// runtimeAddress is the libtpu runtime metrics service of the workload.
	runtimeAddress = "localhost:8431"
	// vbarPollInterval bounds the VBAR polling rate: Google reports that frequent
	// GetHostMetrics calls can crash the VBAR control agent during device resets.
	// Between polls, TensorCore and bandwidth utilization repeat the last poll.
	vbarPollInterval = 60 * time.Second
	// instanceIDURL is the GCE metadata endpoint of the instance ID, from which the
	// device plugin builds chip IDs ("<instance id>-<chip index>").
	instanceIDURL = "http://169.254.169.254/computeMetadata/v1/instance/id"
)

// vbarMetricTypes maps VBAR utilization metric types to device plugin series names.
var vbarMetricTypes = map[uint64]string{1: "tensorcore_utilization", 2: "memory_bandwidth_utilization"}

// runtimeMetrics are the libtpu runtime metric names, with their device plugin series names.
var runtimeMetrics = [][2]string{
	{"tpu.runtime.tensorcore.dutycycle.percent", "duty_cycle"},
	{"tpu.runtime.hbm.memory.usage.bytes", "memory_used"},
	{"tpu.runtime.hbm.memory.total.bytes", "memory_total"},
}

// hostSource reads TPU metrics on a TPU VM, where there is no device plugin.
type hostSource struct {
	instanceID    string // instanceID is read on the first run, as the metadata server may not be ready at startup
	instanceIDURL string
	vbar, runtime *grpc.ClientConn
	lastVBARPoll  time.Time
	lastVBAR      []map[string]float64 // lastVBAR holds the last VBAR series, by chip index
	lastVBARErr   error                // lastVBARErr is the error of the last VBAR poll, returned until the next one
}

func newHostSource(instanceIDURL, vbarAddr, runtimeAddr string) (*hostSource, error) {
	creds := grpc.WithTransportCredentials(insecure.NewCredentials())
	vbar, err := grpc.NewClient(vbarAddr, creds)
	if err != nil {
		return nil, err
	}
	runtime, err := grpc.NewClient(runtimeAddr, creds)
	if err != nil {
		_ = vbar.Close()
		return nil, err
	}
	return &hostSource{instanceIDURL: instanceIDURL, vbar: vbar, runtime: runtime}, nil
}

// read returns the chips of the host. Runtime series are missing when no workload runs.
func (h *hostSource) read() ([]*chip, error) {
	if h.instanceID == "" {
		id, err := readInstanceID(h.instanceIDURL)
		if err != nil {
			return nil, fmt.Errorf("read the GCE instance ID: %w", err)
		}
		h.instanceID = id
	}
	// One deadline bounds the calls of a run, so a stalled service cannot outlast the interval.
	ctx, cancel := context.WithTimeout(context.Background(), scrapeTimeout)
	defer cancel()
	if time.Since(h.lastVBARPoll) >= vbarPollInterval {
		// A failed poll counts too: retrying on every run could crash the VBAR agent.
		h.lastVBARPoll = time.Now()
		resp, err := invoke(ctx, h.vbar, "/libtpu.tpunetd.VBARControl/GetHostMetrics", nil)
		h.lastVBAR, h.lastVBARErr = parseHostMetrics(resp), err
	}
	if h.lastVBARErr != nil {
		return nil, fmt.Errorf("VBAR GetHostMetrics: %w", h.lastVBARErr)
	}
	chips := make([]*chip, len(h.lastVBAR))
	for i, series := range h.lastVBAR {
		chips[i] = &chip{id: h.instanceID + "-" + strconv.Itoa(i), node: maps.Clone(series), workloads: map[workloadKey]map[string]float64{}}
	}
	// libtpu device ordinals are only known to match the VBAR chip on single-chip hosts.
	if len(chips) == 1 {
		for _, m := range runtimeMetrics {
			resp, err := invoke(ctx, h.runtime, "/tpu.monitoring.runtime.RuntimeMetricService/GetRuntimeMetric", protowire.AppendString(protowire.AppendTag(nil, 1, protowire.BytesType), m[0]))
			if ctx.Err() != nil {
				if logLimitCheck.ShouldLog() {
					log.Warnf("libtpu runtime metrics at %s did not answer in time: %s", runtimeAddress, err)
				}
				break
			}
			if status.Code(err) == codes.Unavailable {
				break // no workload holds the chip
			}
			if err != nil {
				continue // libtpu does not export this metric
			}
			if values := parseRuntimeMetric(resp); len(values) == 1 {
				if v, ok := values[0]; ok {
					chips[0].node[m[1]] = v
				}
			}
		}
	}
	return chips, nil
}

func (h *hostSource) close() {
	_ = h.vbar.Close()
	_ = h.runtime.Close()
}

// parseHostMetrics decodes a VBAR GetHostMetrics response: one entry per chip (field 1),
// each with utilization metrics (field 2: type varint in field 1, float32 in field 2) and
// an opaque chip ID (field 3), which orders the chips. Entries without an ID are dropped.
func parseHostMetrics(b []byte) []map[string]float64 {
	type entry struct {
		id     string
		series map[string]float64
	}
	var entries []entry
	for _, e := range bytesFields(b, 1) {
		series := map[string]float64{}
		for _, metric := range bytesFields(e, 2) {
			typ, _ := numberField(metric, 1, protowire.VarintType)
			v, ok := numberField(metric, 2, protowire.Fixed32Type)
			if name, known := vbarMetricTypes[typ]; known && ok {
				series[name] = float64(math.Float32frombits(uint32(v)))
			}
		}
		if id := firstBytes(e, 3); len(id) > 0 {
			entries = append(entries, entry{string(id), series})
		}
	}
	slices.SortFunc(entries, func(a, b entry) int { return strings.Compare(a.id, b.id) })
	chips := make([]map[string]float64, len(entries))
	for i, e := range entries {
		chips[i] = e.series
	}
	return chips
}

// parseRuntimeMetric decodes a libtpu MetricResponse into values by device ordinal:
// metric (1) > metrics (3) > {attribute (1) > value (2) > int_attr (3); gauge (3) > as_double (1) | as_int (2)}.
func parseRuntimeMetric(b []byte) map[int]float64 {
	values := map[int]float64{}
	for _, metric := range bytesFields(firstBytes(b, 1), 3) {
		device, ok := numberField(firstBytes(firstBytes(metric, 1), 2), 3, protowire.VarintType)
		if !ok {
			continue
		}
		gauge := firstBytes(metric, 3)
		if v, ok := numberField(gauge, 1, protowire.Fixed64Type); ok {
			values[int(device)] = math.Float64frombits(v)
		} else if v, ok := numberField(gauge, 2, protowire.VarintType); ok {
			values[int(device)] = float64(int64(v))
		}
	}
	return values
}

// scan calls fn with the wire type and encoded value of each field num of message b.
// A malformed message ends the scan.
func scan(b []byte, num protowire.Number, fn func(protowire.Type, []byte)) {
	for len(b) > 0 {
		n, typ, l := protowire.ConsumeTag(b)
		if l < 0 {
			return
		}
		b = b[l:]
		m := protowire.ConsumeFieldValue(n, typ, b)
		if m < 0 {
			return
		}
		if n == num {
			fn(typ, b[:m])
		}
		b = b[m:]
	}
}

func bytesFields(b []byte, num protowire.Number) (out [][]byte) {
	scan(b, num, func(typ protowire.Type, v []byte) {
		if typ == protowire.BytesType {
			s, _ := protowire.ConsumeBytes(v)
			out = append(out, s)
		}
	})
	return out
}

func firstBytes(b []byte, num protowire.Number) []byte {
	if v := bytesFields(b, num); len(v) > 0 {
		return v[0]
	}
	return nil
}

// numberField returns the last value of field num with the varint, fixed32 or fixed64 wire type want.
func numberField(b []byte, num protowire.Number, want protowire.Type) (v uint64, ok bool) {
	scan(b, num, func(typ protowire.Type, raw []byte) {
		if typ != want {
			return
		}
		switch typ {
		case protowire.VarintType:
			v, _ = protowire.ConsumeVarint(raw)
		case protowire.Fixed32Type:
			x, _ := protowire.ConsumeFixed32(raw)
			v = uint64(x)
		case protowire.Fixed64Type:
			v, _ = protowire.ConsumeFixed64(raw)
		}
		ok = true
	})
	return v, ok
}

// rawCodec sends and receives already encoded protobuf messages.
type rawCodec struct{}

func (rawCodec) Marshal(v any) ([]byte, error)      { return v.([]byte), nil }
func (rawCodec) Unmarshal(data []byte, v any) error { *(v.(*[]byte)) = data; return nil }
func (rawCodec) Name() string                       { return "proto" }

func invoke(ctx context.Context, conn *grpc.ClientConn, method string, req []byte) ([]byte, error) {
	var resp []byte
	err := conn.Invoke(ctx, method, req, &resp, grpc.ForceCodec(rawCodec{}))
	return resp, err
}

// readInstanceID returns the GCE instance ID from the metadata server.
func readInstanceID(url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := (&http.Client{Timeout: scrapeTimeout}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if id := strings.TrimSpace(string(body)); id != "" || err != nil {
		return id, err
	}
	return "", errors.New("empty instance ID")
}
