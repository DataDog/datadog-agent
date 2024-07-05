package trace

import (
	"context"

	compression "github.com/DataDog/datadog-agent/comp/trace/compression/def"
	gzip "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"
	zstd "github.com/DataDog/datadog-agent/comp/trace/compression/impl-zstd"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-go/v5/statsd"
)

type TraceAgent interface {
	Run()
	Cancel()
	FlushSync()
	Process(p *api.Payload)
	SetGlobalTagsUnsafe(map[string]string)
	SetTargetTPS(float64)
	SetSpanModifier(agent.SpanModifier)
	GetSpanModifier() agent.SpanModifier
}

type traceAgent struct {
	ta     *agent.Agent
	cancel context.CancelFunc
}

func NewTraceAgent(tc *config.AgentConfig, lambdaSpanChan chan<- *pb.Span, coldStartSpanId uint64) TraceAgent {
	var compressor compression.Component
	if tc.HasFeature("zstd-encoding") {
		compressor = zstd.NewComponent()
	} else {
		compressor = gzip.NewComponent()
	}
	context, cancel := context.WithCancel(context.Background())
	ta := agent.NewAgent(
		context,
		tc,
		telemetry.NewNoopCollector(),
		&statsd.NoOpClient{},
		compressor,
	)
	ta.SpanModifier = &spanModifier{
		coldStartSpanId: coldStartSpanId,
		lambdaSpanChan:  lambdaSpanChan,
		ddOrigin:        getDDOrigin(),
	}
	ta.DiscardSpan = filterSpanFromLambdaLibraryOrRuntime
	go ta.Run()
	return &traceAgent{
		ta:     ta,
		cancel: cancel,
	}
}

func (t *traceAgent) Run() {
	t.ta.Run()
}

func (t *traceAgent) Cancel() {
	t.cancel()
}

func (t *traceAgent) FlushSync() {
	t.ta.FlushSync()
}

func (t *traceAgent) Process(p *api.Payload) {
	t.ta.Process(p)
}

func (t *traceAgent) SetGlobalTagsUnsafe(tags map[string]string) {
	t.ta.SetGlobalTagsUnsafe(tags)
	t.ta.SpanModifier.SetTags(tags)
}

func (t *traceAgent) SetTargetTPS(tps float64) {
	t.ta.PrioritySampler.UpdateTargetTPS(tps)
}

func (t *traceAgent) SetSpanModifier(sm agent.SpanModifier) {
	t.ta.SpanModifier = sm
}

func (t *traceAgent) GetSpanModifier() agent.SpanModifier {
	return t.ta.SpanModifier
}

type noopTraceAgent struct{}

func (t noopTraceAgent) Run()                                       {}
func (t noopTraceAgent) Cancel()                                    {}
func (t noopTraceAgent) FlushSync()                                 {}
func (t noopTraceAgent) Process(p *api.Payload)                     {}
func (t noopTraceAgent) SetGlobalTagsUnsafe(tags map[string]string) {}
func (t noopTraceAgent) SetTargetTPS(tps float64)                   {}
func (t noopTraceAgent) SetSpanModifier(sm agent.SpanModifier)      {}
func (t noopTraceAgent) GetSpanModifier() agent.SpanModifier {
	return noopSpanModifier{}
}

type noopSpanModifier struct{}

func (noopSpanModifier) ModifySpan(chunk *pb.TraceChunk, span *pb.Span) {}
func (noopSpanModifier) SetTags(tags map[string]string)                 {}
