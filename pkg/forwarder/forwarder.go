package forwarder

import (
	"bytes"
	"github.com/DataDog/datadog-agent/pb"
	log "github.com/cihub/seelog"
	"github.com/golang/protobuf/proto"
	"net/http"
	"time"
)

type Forwarder struct {
	transactionChan         chan *Transaction
	requeuedTransactionChan chan *Transaction
	client                  *http.Client
}

func NewForwarder() *Forwarder {

	httpClient := &http.Client{
		Timeout: 20 * time.Second,
	}
	return &Forwarder{
		transactionChan:         make(chan *Transaction),
		requeuedTransactionChan: make(chan *Transaction),
		client:                  httpClient,
	}
}

type Transaction struct {
	Host      string
	Endpoint  string
	APIKey    string
	Headers   map[string][]string
	Payload   *[]byte
	NotBefore time.Time
}

func NewMetricsTransaction() *Transaction {
	return &Transaction{
		Host:     "http://requestb.in/1o05yzr1", //https://6-0-0.agent.datadoghq.com",
		Endpoint: "",                            //"/api/v2/series",
	}
}

func (f *Forwarder) SubmitSeries(payload *pb.MetricsPayload) {

	// This should be completely async
	go func() {

		serializedData, _ := proto.Marshal(payload)

		trans := NewMetricsTransaction()
		trans.Payload = &serializedData

		f.transactionChan <- trans

	}()

}

func (f *Forwarder) Start() {

	log.Info("Starting forwarder")
	go f.processRegularTransactions()
	go f.processRequeuedTransactions()
	log.Info("Forwarder started")
}

func (f *Forwarder) Stop() {

	log.Info("STOP IS NOT IMPLEMENTED YET.")
}

func (f *Forwarder) processRegularTransactions() {

	for {
		trans := <-f.transactionChan
		f.processTransaction(trans)
	}

}

func (f *Forwarder) processRequeuedTransactions() {

	for {
		trans := <-f.requeuedTransactionChan
		if time.Now().Before(trans.NotBefore) {
			// Not quite ready for this one yet, let's send it back to the channel
			f.requeuedTransactionChan <- trans
			continue
		}

		f.processTransaction(trans)

	}

}

func (f *Forwarder) processTransaction(trans *Transaction) {

	url := trans.Host + trans.Endpoint

	log.Info("PAYLOAD IS")
	log.Info(*trans.Payload)
	log.Info("$$$$$$$$")
	bytes.NewReader(*trans.Payload)

	req, err := http.NewRequest("POST", url, bytes.NewReader(*trans.Payload))
	req.Header = trans.Headers

	log.Infof("Posting req %v: %v", req, err)

	f.client.Do(req)

}
