package interpreter

import (
	"github.com/StackVista/stackstate-agent/pkg/trace/config"
	interpreterConfig "github.com/StackVista/stackstate-agent/pkg/trace/interpreter/config"
	"github.com/StackVista/stackstate-agent/pkg/trace/interpreter/interpreters"
	"github.com/StackVista/stackstate-agent/pkg/trace/interpreter/model"
	"github.com/StackVista/stackstate-agent/pkg/trace/pb"
)

// SpanInterpreterEngine type is used to setup the span interpreters
type SpanInterpreterEngine struct {
	SpanInterpreterEngineContext
	DefaultSpanInterpreter *interpreters.DefaultSpanInterpreter
	SourceInterpreters     map[string]interpreters.SourceInterpreter
	TypeInterpreters       map[string]interpreters.TypeInterpreter
}

// MakeSpanInterpreterEngine creates a SpanInterpreterEngine given the config and interpreters
func MakeSpanInterpreterEngine(config *interpreterConfig.Config, typeIns map[string]interpreters.TypeInterpreter, sourceIns map[string]interpreters.SourceInterpreter) *SpanInterpreterEngine {
	return &SpanInterpreterEngine{
		DefaultSpanInterpreter:       interpreters.MakeDefaultSpanInterpreter(config),
		SpanInterpreterEngineContext: MakeSpanInterpreterEngineContext(config),
		SourceInterpreters:           sourceIns,
		TypeInterpreters:             typeIns,
	}
}

// NewSpanInterpreterEngine creates a SpanInterpreterEngine given the config and the default interpreters
func NewSpanInterpreterEngine(agentConfig *config.AgentConfig) *SpanInterpreterEngine {
	interpreterConfig := agentConfig.InterpreterConfig
	typeIns := make(map[string]interpreters.TypeInterpreter, 0)
	typeIns[interpreters.ProcessSpanInterpreterName] = interpreters.MakeProcessSpanInterpreter(interpreterConfig)
	typeIns[interpreters.SQLSpanInterpreterName] = interpreters.MakeSQLSpanInterpreter(interpreterConfig)
	sourceIns := make(map[string]interpreters.SourceInterpreter, 0)
	sourceIns[interpreters.TraefikSpanInterpreterSpan] = interpreters.MakeTraefikInterpreter(interpreterConfig)

	return MakeSpanInterpreterEngine(interpreterConfig, typeIns, sourceIns)
}

// Interpret interprets spans using the configured SpanInterpreterEngine
func (se *SpanInterpreterEngine) Interpret(span *pb.Span) *pb.Span {

	// check if span is pre-interpreted by the trace client
	if _, found := span.Meta["span.serviceURN"]; found {
		return span
	}

	span = se.DefaultSpanInterpreter.Interpret(span)

	meta, err := se.extractSpanMetadata(span)
	// no metadata, let's look for the span's source.
	if err != nil {
		if source, found := span.Meta["source"]; found {
			// interpret the source if we have a interpreter.
			if interpreter, found := se.SourceInterpreters[source]; found {
				span = interpreter.Interpret(span)
			}
		}
	} else {
		// process different span types
		spanWithMeta := &model.SpanWithMeta{Span: span, SpanMetadata: meta}

		// interpret the type if we have a interpreter, otherwise run it through the process interpreter.
		if interpreter, found := se.TypeInterpreters[meta.Type]; found {
			span = interpreter.Interpret(spanWithMeta)
		} else {
			span = se.TypeInterpreters["process"].Interpret(spanWithMeta)
		}
	}
	// we mutate so we return the "same" span
	return span
}
