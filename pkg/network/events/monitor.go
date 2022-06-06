package events

import (
	"sync"
	"sync/atomic"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

var theMonitor atomic.Value
var once sync.Once
var initErr error

// Init initializes the events package
func Init() error {
	once.Do(func() {
		var m *eventMonitor
		m, initErr = newEventMonitor()
		if initErr == nil {
			theMonitor.Store(m)
		}
	})

	return initErr
}

// HandlerFunc is the prototype for an event handler callback for process events
type HandlerFunc func(*model.ProcessCacheEntry)

func RegisterHandler(handler HandlerFunc) {
	m := theMonitor.Load().(*eventMonitor)
	m.RegisterHandler(handler)
}

type eventHandlerWrapper struct{}

func (h *eventHandlerWrapper) HandleEvent(ev *sprobe.Event) {
	m := theMonitor.Load()
	if m != nil {
		m.(*eventMonitor).HandleEvent(ev)
	}
}

func (h *eventHandlerWrapper) HandleCustomEvent(rule *rules.Rule, event *sprobe.CustomEvent) {
	m := theMonitor.Load()
	if m != nil {
		m.(*eventMonitor).HandleCustomEvent(rule, event)
	}
}

var _eventHandlerWrapper = &eventHandlerWrapper{}

// Handler returns an event handler to handle events from the runtime security module
func Handler() sprobe.EventHandler {
	return _eventHandlerWrapper
}

type eventMonitor struct {
	handlers []HandlerFunc
}

func newEventMonitor() (*eventMonitor, error) {
	return &eventMonitor{}, nil
}

func (e *eventMonitor) HandleEvent(ev *sprobe.Event) {
	_ = ev.ResolveProcessEnvp(&ev.ProcessContext.Process)

	entry := ev.ResolveProcessCacheEntry()
	if entry == nil {
		return
	}

	for _, h := range e.handlers {
		h(entry)
	}

}

func (e *eventMonitor) HandleCustomEvent(rule *rules.Rule, event *sprobe.CustomEvent) {
}

func (e *eventMonitor) RegisterHandler(handler HandlerFunc) {
	if handler != nil {
		e.handlers = append(e.handlers, handler)
	}
}
