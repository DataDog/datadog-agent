package writer

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/trace/writer/config"
)

var _ payloadSender = (*multiSender)(nil)

// multiSender is an implementation of payloadSender which forwards any
// received payload to multiple payloadSenders, funnelling incoming monitor
// events.
type multiSender struct {
	senders []payloadSender
	mwg     sync.WaitGroup    // monitor funnel waitgroup
	mch     chan monitorEvent // monitor funneling channel
}

// newMultiSender returns a new payloadSender which forwards all sent payloads to all
// the given endpoints, as well as funnels all monitoring channels.
func newMultiSender(endpoints []endpoint, cfg config.QueuablePayloadSenderConf) payloadSender {
	if len(endpoints) == 1 {
		return newSender(endpoints[0], cfg)
	}
	senders := make([]payloadSender, len(endpoints))
	for i, e := range endpoints {
		senders[i] = newSender(e, cfg)
	}
	return &multiSender{
		senders: senders,
		mch:     make(chan monitorEvent, len(senders)),
	}
}

// Start starts all senders.
func (w *multiSender) Start() {
	for _, sender := range w.senders {
		sender.Start()
	}
	for _, sender := range w.senders {
		w.mwg.Add(1)
		go func(ch <-chan monitorEvent) {
			defer w.mwg.Done()
			for event := range ch {
				w.mch <- event
			}
		}(sender.Monitor())
	}
}

// Stop stops all senders.
func (w *multiSender) Stop() {
	for _, sender := range w.senders {
		sender.Stop()
	}
	w.mwg.Wait()
	close(w.mch)
}

// Send forwards the payload to all registered senders.
func (w *multiSender) Send(p *payload) {
	for _, sender := range w.senders {
		sender.Send(p)
	}
}

func (w *multiSender) Monitor() <-chan monitorEvent { return w.mch }

// Run implements payloadSender.
func (w *multiSender) Run() { /* no-op */ }

func (w *multiSender) setEndpoint(endpoint endpoint) {
	for _, sender := range w.senders {
		sender.setEndpoint(endpoint)
	}
}
