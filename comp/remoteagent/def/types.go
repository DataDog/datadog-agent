package remoteagent

import "context"

type StatusRequests <-chan *Request[*StatusData]
type FlareRequests <-chan *Request[*FlareData]

type Request[T any] struct {
	context context.Context
	dataOut chan<- T
}

func NewRequest[T any](context context.Context, dataOut chan<- T) *Request[T] {
	return &Request[T]{
		context: context,
		dataOut: dataOut,
	}
}

func (r *Request[T]) Context() context.Context {
	return r.context
}

func (r *Request[T]) Fulfill(data T) {
	r.dataOut <- data
}

type StatusSection map[string]string

type StatusData struct {
	AgentId       string
	FailureReason string
	MainSection   StatusSection
	NamedSections map[string]StatusSection
}

type FlareData struct {
	AgentId string
	Files   map[string][]byte
}
