package agent

import (
	"bytes"
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/event"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/trace/writer"
	ddlog "github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/tinylib/msgp/msgp"
)

type mockSamplerEngine struct {
	engine sampler.Engine
}

func newMockSampler(wantSampled bool, wantRate float64) *Sampler {
	return &Sampler{engine: testutil.NewMockEngine(wantSampled, wantRate)}
}

// Test to make sure that the joined effort of the quantizer and truncator, in that order, produce the
// desired string
func TestFormatTrace(t *testing.T) {
	assert := assert.New(t)
	resource := "SELECT name FROM people WHERE age = 42"
	rep := strings.Repeat(" AND age = 42", 5000)
	resource = resource + rep
	testTrace := pb.Trace{
		&pb.Span{
			Resource: resource,
			Type:     "sql",
		},
	}
	result := formatTrace(testTrace)[0]

	assert.Equal(5000, len(result.Resource))
	assert.NotEqual("Non-parsable SQL query", result.Resource)
	assert.NotContains(result.Resource, "42")
	assert.Contains(result.Resource, "SELECT name FROM people WHERE age = ?")

	assert.Equal(5003, len(result.Meta["sql.query"])) // Ellipsis added in quantizer
	assert.NotEqual("Non-parsable SQL query", result.Meta["sql.query"])
	assert.NotContains(result.Meta["sql.query"], "42")
	assert.Contains(result.Meta["sql.query"], "SELECT name FROM people WHERE age = ?")
}

func TestProcess(t *testing.T) {
	t.Run("Replacer", func(t *testing.T) {
		// Ensures that for "sql" type spans:
		// • obfuscator runs before replacer
		// • obfuscator obfuscates both resource and "sql.query" tag
		// • resulting resource is obfuscated with replacements applied
		// • resulting "sql.query" tag is obfuscated with no replacements applied
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		cfg.ReplaceTags = []*config.ReplaceRule{{
			Name: "resource.name",
			Re:   regexp.MustCompile("AND.*"),
			Repl: "...",
		}}
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewAgent(ctx, cfg)
		defer cancel()

		now := time.Now()
		span := &pb.Span{
			Resource: "SELECT name FROM people WHERE age = 42 AND extra = 55",
			Type:     "sql",
			Start:    now.Add(-time.Second).UnixNano(),
			Duration: (500 * time.Millisecond).Nanoseconds(),
		}
		agnt.Process(&api.Trace{
			Spans:  pb.Trace{span},
			Source: &info.Tags{},
		})

		assert := assert.New(t)
		assert.Equal("SELECT name FROM people WHERE age = ? ...", span.Resource)
		assert.Equal("SELECT name FROM people WHERE age = ? AND extra = ?", span.Meta["sql.query"])
	})

	t.Run("Blacklister", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		cfg.Ignore["resource"] = []string{"^INSERT.*"}
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewAgent(ctx, cfg)
		defer cancel()

		now := time.Now()
		spanValid := &pb.Span{
			Resource: "SELECT name FROM people WHERE age = 42 AND extra = 55",
			Type:     "sql",
			Start:    now.Add(-time.Second).UnixNano(),
			Duration: (500 * time.Millisecond).Nanoseconds(),
		}
		spanInvalid := &pb.Span{
			Resource: "INSERT INTO db VALUES (1, 2, 3)",
			Type:     "sql",
			Start:    now.Add(-time.Second).UnixNano(),
			Duration: (500 * time.Millisecond).Nanoseconds(),
		}

		stats := agnt.Receiver.Stats.GetTagStats(info.Tags{})
		assert := assert.New(t)

		agnt.Process(&api.Trace{
			Spans:  pb.Trace{spanValid},
			Source: &info.Tags{},
		})
		assert.EqualValues(0, stats.TracesFiltered)
		assert.EqualValues(0, stats.SpansFiltered)

		agnt.Process(&api.Trace{
			Spans:  pb.Trace{spanInvalid, spanInvalid},
			Source: &info.Tags{},
		})
		assert.EqualValues(1, stats.TracesFiltered)
		assert.EqualValues(2, stats.SpansFiltered)
	})

	t.Run("Stats/Priority", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test"
		ctx, cancel := context.WithCancel(context.Background())
		agnt := NewAgent(ctx, cfg)
		defer cancel()

		now := time.Now()
		for _, key := range []sampler.SamplingPriority{
			sampler.PriorityNone,
			sampler.PriorityUserDrop,
			sampler.PriorityUserDrop,
			sampler.PriorityAutoDrop,
			sampler.PriorityAutoDrop,
			sampler.PriorityAutoDrop,
			sampler.PriorityAutoKeep,
			sampler.PriorityAutoKeep,
			sampler.PriorityAutoKeep,
			sampler.PriorityAutoKeep,
			sampler.PriorityUserKeep,
			sampler.PriorityUserKeep,
			sampler.PriorityUserKeep,
			sampler.PriorityUserKeep,
			sampler.PriorityUserKeep,
		} {
			span := &pb.Span{
				Resource: "SELECT name FROM people WHERE age = 42 AND extra = 55",
				Type:     "sql",
				Start:    now.Add(-time.Second).UnixNano(),
				Duration: (500 * time.Millisecond).Nanoseconds(),
				Metrics:  map[string]float64{},
			}
			if key != sampler.PriorityNone {
				sampler.SetSamplingPriority(span, key)
			}
			agnt.Process(&api.Trace{
				Spans:  pb.Trace{span},
				Source: &info.Tags{},
			})
		}

		stats := agnt.Receiver.Stats.GetTagStats(info.Tags{})
		assert.EqualValues(t, 1, stats.TracesPriorityNone)
		assert.EqualValues(t, 2, stats.TracesPriorityNeg)
		assert.EqualValues(t, 3, stats.TracesPriority0)
		assert.EqualValues(t, 4, stats.TracesPriority1)
		assert.EqualValues(t, 5, stats.TracesPriority2)
	})
}

func TestSampling(t *testing.T) {
	for name, tt := range map[string]struct {
		// hasErrors will be true if the input trace should have errors
		// hasPriority will be true if the input trace should have sampling priority set
		hasErrors, hasPriority bool

		// scoreRate, scoreErrorRate, priorityRate are the rates used by the mock samplers
		scoreRate, scoreErrorRate, priorityRate float64

		// scoreSampled, scoreErrorSampled, prioritySampled are the sample decisions of the mock samplers
		scoreSampled, scoreErrorSampled, prioritySampled bool

		// wantRate and wantSampled are the expected result
		wantRate    float64
		wantSampled bool
	}{
		"score and priority rate": {
			hasPriority:  true,
			scoreRate:    0.5,
			priorityRate: 0.6,
			wantRate:     sampler.CombineRates(0.5, 0.6),
		},
		"score only rate": {
			scoreRate:    0.5,
			priorityRate: 0.1,
			wantRate:     0.5,
		},
		"error and priority rate": {
			hasErrors:      true,
			hasPriority:    true,
			scoreErrorRate: 0.8,
			priorityRate:   0.2,
			wantRate:       sampler.CombineRates(0.8, 0.2),
		},
		"score not sampled decision": {
			scoreSampled: false,
			wantSampled:  false,
		},
		"score sampled decision": {
			scoreSampled: true,
			wantSampled:  true,
		},
		"score sampled priority not sampled": {
			hasPriority:     true,
			scoreSampled:    true,
			prioritySampled: false,
			wantSampled:     true,
		},
		"score not sampled priority sampled": {
			hasPriority:     true,
			scoreSampled:    false,
			prioritySampled: true,
			wantSampled:     true,
		},
		"score sampled priority sampled": {
			hasPriority:     true,
			scoreSampled:    true,
			prioritySampled: true,
			wantSampled:     true,
		},
		"score and priority not sampled": {
			hasPriority:     true,
			scoreSampled:    false,
			prioritySampled: false,
			wantSampled:     false,
		},
		"error not sampled decision": {
			hasErrors:         true,
			scoreErrorSampled: false,
			wantSampled:       false,
		},
		"error sampled decision": {
			hasErrors:         true,
			scoreErrorSampled: true,
			wantSampled:       true,
		},
		"error sampled priority not sampled": {
			hasErrors:         true,
			hasPriority:       true,
			scoreErrorSampled: true,
			prioritySampled:   false,
			wantSampled:       true,
		},
		"error not sampled priority sampled": {
			hasErrors:         true,
			hasPriority:       true,
			scoreErrorSampled: false,
			prioritySampled:   true,
			wantSampled:       true,
		},
		"error sampled priority sampled": {
			hasErrors:         true,
			hasPriority:       true,
			scoreErrorSampled: true,
			prioritySampled:   true,
			wantSampled:       true,
		},
		"error and priority not sampled": {
			hasErrors:         true,
			hasPriority:       true,
			scoreErrorSampled: false,
			prioritySampled:   false,
			wantSampled:       false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			a := &Agent{
				ScoreSampler:       newMockSampler(tt.scoreSampled, tt.scoreRate),
				ErrorsScoreSampler: newMockSampler(tt.scoreErrorSampled, tt.scoreErrorRate),
				PrioritySampler:    newMockSampler(tt.prioritySampled, tt.priorityRate),
			}
			root := &pb.Span{
				Service:  "serv1",
				Start:    time.Now().UnixNano(),
				Duration: (100 * time.Millisecond).Nanoseconds(),
				Metrics:  map[string]float64{},
			}

			if tt.hasErrors {
				root.Error = 1
			}
			pt := ProcessedTrace{Trace: pb.Trace{root}, Root: root}
			if tt.hasPriority {
				sampler.SetSamplingPriority(pt.Root, 1)
			}

			sampled, rate := a.runSamplers(pt)
			assert.EqualValues(t, tt.wantRate, rate)
			assert.EqualValues(t, tt.wantSampled, sampled)
		})
	}
}

func TestEventProcessorFromConf(t *testing.T) {
	if _, ok := os.LookupEnv("INTEGRATION"); !ok {
		t.Skip("set INTEGRATION environment variable to run")
	}
	if testing.Short() {
		return
	}
	testMaxEPS := 100.
	rateByServiceAndName := map[string]map[string]float64{
		"serviceA": {
			"opA": 0,
			"opC": 1,
		},
		"serviceB": {
			"opB": 0.5,
		},
	}
	rateByService := map[string]float64{
		"serviceA": 1,
		"serviceC": 0.5,
		"serviceD": 1,
	}

	for _, testCase := range []eventProcessorTestCase{
		// Name: <extractor>/<maxeps situation>/priority
		{name: "none/below/none", intakeSPS: 100, serviceName: "serviceE", opName: "opA", extractionRate: -1, priority: sampler.PriorityNone, expectedEPS: 0, deltaPct: 0, duration: 10 * time.Second},
		{name: "metric/below/none", intakeSPS: 100, serviceName: "serviceD", opName: "opA", extractionRate: 0.5, priority: sampler.PriorityNone, expectedEPS: 50, deltaPct: 0.1, duration: 10 * time.Second},
		{name: "metric/above/none", intakeSPS: 200, serviceName: "serviceD", opName: "opA", extractionRate: 1, priority: sampler.PriorityNone, expectedEPS: 100, deltaPct: 0.5, duration: 60 * time.Second},
		{name: "fixed/below/none", intakeSPS: 100, serviceName: "serviceB", opName: "opB", extractionRate: -1, priority: sampler.PriorityNone, expectedEPS: 50, deltaPct: 0.1, duration: 10 * time.Second},
		{name: "fixed/above/none", intakeSPS: 200, serviceName: "serviceA", opName: "opC", extractionRate: -1, priority: sampler.PriorityNone, expectedEPS: 100, deltaPct: 0.5, duration: 60 * time.Second},
		{name: "fixed/above/autokeep", intakeSPS: 200, serviceName: "serviceA", opName: "opC", extractionRate: -1, priority: sampler.PriorityAutoKeep, expectedEPS: 100, deltaPct: 0.5, duration: 60 * time.Second},
		{name: "metric/above/autokeep", intakeSPS: 200, serviceName: "serviceD", opName: "opA", extractionRate: 1, priority: sampler.PriorityAutoKeep, expectedEPS: 100, deltaPct: 0.5, duration: 60 * time.Second},
		// UserKeep traces allows overflow of EPS
		{name: "metric/above/userkeep", intakeSPS: 200, serviceName: "serviceD", opName: "opA", extractionRate: 1, priority: sampler.PriorityUserKeep, expectedEPS: 200, deltaPct: 0.1, duration: 10 * time.Second},
		{name: "agent/above/userkeep", intakeSPS: 200, serviceName: "serviceA", opName: "opC", extractionRate: -1, priority: sampler.PriorityUserKeep, expectedEPS: 200, deltaPct: 0.1, duration: 10 * time.Second},

		// Overrides (Name: <extractor1>/override/<extractor2>)
		{name: "metric/override/fixed", intakeSPS: 100, serviceName: "serviceA", opName: "opA", extractionRate: 1, priority: sampler.PriorityNone, expectedEPS: 100, deltaPct: 0.1, duration: 10 * time.Second},
		// Legacy should never be considered if fixed rate is being used.
		{name: "fixed/override/legacy", intakeSPS: 100, serviceName: "serviceA", opName: "opD", extractionRate: -1, priority: sampler.PriorityNone, expectedEPS: 0, deltaPct: 0, duration: 10 * time.Second},
	} {
		testEventProcessorFromConf(t, &config.AgentConfig{
			MaxEPS:                      testMaxEPS,
			AnalyzedSpansByService:      rateByServiceAndName,
			AnalyzedRateByServiceLegacy: rateByService,
		}, testCase)
	}
}

func TestEventProcessorFromConfLegacy(t *testing.T) {
	if _, ok := os.LookupEnv("INTEGRATION"); !ok {
		t.Skip("set INTEGRATION environment variable to run")
	}

	testMaxEPS := 100.

	rateByService := map[string]float64{
		"serviceA": 1,
		"serviceC": 0.5,
		"serviceD": 1,
	}

	for _, testCase := range []eventProcessorTestCase{
		// Name: <extractor>/<maxeps situation>/priority
		{name: "none/below/none", intakeSPS: 100, serviceName: "serviceE", opName: "opA", extractionRate: -1, priority: sampler.PriorityNone, expectedEPS: 0, deltaPct: 0, duration: 10 * time.Second},
		{name: "legacy/below/none", intakeSPS: 100, serviceName: "serviceC", opName: "opB", extractionRate: -1, priority: sampler.PriorityNone, expectedEPS: 50, deltaPct: 0.1, duration: 10 * time.Second},
		{name: "legacy/above/none", intakeSPS: 200, serviceName: "serviceD", opName: "opC", extractionRate: -1, priority: sampler.PriorityNone, expectedEPS: 100, deltaPct: 0.5, duration: 60 * time.Second},
		{name: "legacy/above/autokeep", intakeSPS: 200, serviceName: "serviceD", opName: "opC", extractionRate: -1, priority: sampler.PriorityAutoKeep, expectedEPS: 100, deltaPct: 0.5, duration: 60 * time.Second},
		// UserKeep traces allows overflow of EPS
		{name: "legacy/above/userkeep", intakeSPS: 200, serviceName: "serviceD", opName: "opC", extractionRate: -1, priority: sampler.PriorityUserKeep, expectedEPS: 200, deltaPct: 0.1, duration: 10 * time.Second},

		// Overrides (Name: <extractor1>/override/<extractor2>)
		{name: "metrics/overrides/legacy", intakeSPS: 100, serviceName: "serviceC", opName: "opC", extractionRate: 1, priority: sampler.PriorityNone, expectedEPS: 100, deltaPct: 0.1, duration: 10 * time.Second},
	} {
		testEventProcessorFromConf(t, &config.AgentConfig{
			MaxEPS:                      testMaxEPS,
			AnalyzedRateByServiceLegacy: rateByService,
		}, testCase)
	}
}

type eventProcessorTestCase struct {
	name           string
	intakeSPS      float64
	serviceName    string
	opName         string
	extractionRate float64
	priority       sampler.SamplingPriority
	expectedEPS    float64
	deltaPct       float64
	duration       time.Duration
}

func testEventProcessorFromConf(t *testing.T, conf *config.AgentConfig, testCase eventProcessorTestCase) {
	t.Run(testCase.name, func(t *testing.T) {
		processor := eventProcessorFromConf(conf)
		processor.Start()

		actualEPS := generateTraffic(processor, testCase.serviceName, testCase.opName, testCase.extractionRate,
			testCase.duration, testCase.intakeSPS, testCase.priority)

		processor.Stop()

		assert.InDelta(t, testCase.expectedEPS, actualEPS, testCase.expectedEPS*testCase.deltaPct)
	})
}

// generateTraffic generates traces every 100ms with enough spans to meet the desired `intakeSPS` (intake spans per
// second). These spans will all have the provided service and operation names and be set as extractable/sampled
// based on the associated rate/%. This traffic generation will run for the specified `duration`.
func generateTraffic(processor *event.Processor, serviceName string, operationName string, extractionRate float64,
	duration time.Duration, intakeSPS float64, priority sampler.SamplingPriority) float64 {
	tickerInterval := 100 * time.Millisecond
	totalSampled := 0
	timer := time.NewTimer(duration)
	eventTicker := time.NewTicker(tickerInterval)
	defer eventTicker.Stop()
	numTicksInSecond := float64(time.Second) / float64(tickerInterval)
	spansPerTick := int(math.Round(float64(intakeSPS) / numTicksInSecond))

Loop:
	for {
		spans := make([]*pb.Span, spansPerTick)
		for i := range spans {
			span := testutil.RandomSpan()
			span.Service = serviceName
			span.Name = operationName
			if extractionRate >= 0 {
				span.Metrics[sampler.KeySamplingRateEventExtraction] = extractionRate
			}
			traceutil.SetTopLevel(span, true)
			spans[i] = span
		}
		root := spans[0]
		if priority != sampler.PriorityNone {
			sampler.SetSamplingPriority(root, priority)
		}

		events, _ := processor.Process(root, spans)
		totalSampled += len(events)

		<-eventTicker.C
		select {
		case <-timer.C:
			// If timer ran out, break out of loop and stop generation
			break Loop
		default:
			// Otherwise, lets generate another
		}
	}
	return float64(totalSampled) / duration.Seconds()
}

func BenchmarkAgentTraceProcessing(b *testing.B) {
	c := config.New()
	c.Endpoints[0].APIKey = "test"

	runTraceProcessingBenchmark(b, c)
}

func BenchmarkAgentTraceProcessingWithFiltering(b *testing.B) {
	c := config.New()
	c.Endpoints[0].APIKey = "test"
	c.Ignore["resource"] = []string{"[0-9]{3}", "foobar", "G.T [a-z]+", "[^123]+_baz"}

	runTraceProcessingBenchmark(b, c)
}

// worst case scenario: spans are tested against multiple rules without any match.
// this means we won't compesate the overhead of filtering by dropping traces
func BenchmarkAgentTraceProcessingWithWorstCaseFiltering(b *testing.B) {
	c := config.New()
	c.Endpoints[0].APIKey = "test"
	c.Ignore["resource"] = []string{"[0-9]{3}", "foobar", "aaaaa?aaaa", "[^123]+_baz"}

	runTraceProcessingBenchmark(b, c)
}

func runTraceProcessingBenchmark(b *testing.B, c *config.AgentConfig) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	ta := NewAgent(ctx, c)
	seelog.UseLogger(seelog.Disabled)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ta.Process(&api.Trace{
			Spans:  testutil.RandomTrace(10, 8),
			Source: &info.Tags{},
		})
	}
}

// Mimicks behaviour of agent Process function
func formatTrace(t pb.Trace) pb.Trace {
	for _, span := range t {
		obfuscate.NewObfuscator(nil).Obfuscate(span)
		Truncate(span)
	}
	return t
}

func BenchmarkThroughput(b *testing.B) {
	env, ok := os.LookupEnv("DD_TRACE_TEST_FOLDER")
	if !ok {
		b.SkipNow()
	}

	ddlog.SetupDatadogLogger(seelog.Disabled, "") // disable logging

	folder := filepath.Join(env, "benchmarks")
	filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
		ext := filepath.Ext(path)
		if ext != ".msgp" {
			return nil
		}
		b.Run(info.Name(), benchThroughput(path))
		return nil
	})
}

func benchThroughput(file string) func(*testing.B) {
	return func(b *testing.B) {
		data, count, err := tracesFromFile(file)
		if err != nil {
			b.Fatal(err)
		}
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "irrelevant"

		ctx, cancelFunc := context.WithCancel(context.Background())
		http.DefaultServeMux = &http.ServeMux{}
		agnt := NewAgent(ctx, cfg)
		defer cancelFunc()

		// start the agent without the trace and stats writers; we will be draining
		// these channels ourselves in the benchmarks, plus we don't want the writers
		// resource usage to show up in the results.
		agnt.spansOut = make(chan *writer.SampledSpans)
		go agnt.Run()

		// wait for receiver to start:
		for {
			resp, err := http.Get("http://localhost:8126/v0.4/traces")
			if err != nil {
				time.Sleep(time.Millisecond)
				continue
			}
			if resp.StatusCode == 400 {
				break
			}
		}

		// drain every other channel to avoid blockage.
		exit := make(chan bool)
		go func() {
			defer close(exit)
			for {
				select {
				case <-agnt.Concentrator.Out:
				case <-exit:
					return
				}
			}
		}()

		b.ResetTimer()
		b.SetBytes(int64(len(data)))

		for i := 0; i < b.N; i++ {
			req, err := http.NewRequest("PUT", "http://localhost:8126/v0.4/traces", bytes.NewReader(data))
			if err != nil {
				b.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/msgpack")
			w := httptest.NewRecorder()

			// create the request by calling directly into the Handler;
			// we are not interested in benchmarking HTTP latency. This
			// also ensures we avoid potential connection failures that
			// would make the benchmarks inconsistent.
			http.DefaultServeMux.ServeHTTP(w, req)
			if w.Code != 200 {
				b.Fatalf("%d: %v", i, w.Body.String())
			}

			var got int
			timeout := time.After(1 * time.Second)
		loop:
			for {
				select {
				case <-agnt.spansOut:
					got++
					if got == count {
						// processed everything!
						break loop
					}
				case <-timeout:
					// taking too long...
					b.Fatalf("time out at %d/%d", got, count)
					break loop
				}
			}
		}

		exit <- true
		<-exit
	}
}

// tracesFromFile extracts raw msgpack data from the given file, modifying each trace
// to have sampling.priority=2 to guarantee consistency. It also returns the amount of
// traces found and any error in obtaining the information.
func tracesFromFile(file string) (raw []byte, count int, err error) {
	if file[0] != '/' {
		file = filepath.Join(os.Getenv("GOPATH"), file)
	}
	in, err := os.Open(file)
	if err != nil {
		return nil, 0, err
	}
	defer in.Close()
	// prepare the traces in this file by adding sampling.priority=2
	// everywhere to ensure consistent sampling assumptions and results.
	var traces pb.Traces
	if err := msgp.Decode(in, &traces); err != nil {
		return nil, 0, err
	}
	for _, t := range traces {
		count++
		for _, s := range t {
			if s.Metrics == nil {
				s.Metrics = map[string]float64{"_sampling_priority_v1": 2}
			} else {
				s.Metrics["_sampling_priority_v1"] = 2
			}
		}
	}
	// re-encode the modified payload
	var data bytes.Buffer
	if err := msgp.Encode(&data, traces); err != nil {
		return nil, 0, err
	}
	return data.Bytes(), count, nil
}
