package forwarder

import (
	"net/http"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// Worker comsumes Transaction (aka transactions) from the Forwarder and
// process them. If the transaction fail to be processed the Worker will send
// it back to the Forwarder to be retried later.
type Worker struct {
	// Client the http client used to processed transactions.
	Client *http.Client
	// InputChan is the channel used to receive transaction from the Forwarder.
	InputChan <-chan Transaction
	// RequeueChan is the channel used to send failed transaction back to the Forwarder.
	RequeueChan chan<- Transaction

	stopChan chan bool
}

// NewWorker returns a new worker to consume Transaction from inputChan
// and push back erroneous ones into requeueChan.
func NewWorker(inputChan chan Transaction, requeueChan chan Transaction) *Worker {

	transport := util.CreateHTTPTransport()

	httpClient := &http.Client{
		Timeout:   config.Datadog.GetDuration("forwarder_timeout") * time.Second,
		Transport: transport,
	}

	return &Worker{
		InputChan:   inputChan,
		RequeueChan: requeueChan,
		stopChan:    make(chan bool),
		Client:      httpClient,
	}
}

// Stop stops the worker.
func (w *Worker) Stop() {
	w.stopChan <- true
}

// Start starts a Worker.
func (w *Worker) Start() {
	go func() {
		for {
			select {
			case t := <-w.InputChan:
				w.process(t)
			case <-w.stopChan:
				return
			}
		}
	}()
}

func (w *Worker) process(t Transaction) {
	if err := t.Process(w.Client); err != nil {
		log.Errorf("Error while processing transaction: %v", err)
		t.Reschedule()
		w.RequeueChan <- t
	}
}
