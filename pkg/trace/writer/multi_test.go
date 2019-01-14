package writer

import (
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/writer/config"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	testStatsClient := &testutil.TestStatsClient{}
	originalClient := metrics.Client
	metrics.Client = testStatsClient
	defer func() {
		metrics.Client = originalClient
	}()
	os.Exit(m.Run())
}

func TestNewMultiSenderFactory(t *testing.T) {
	cfg := config.DefaultQueuablePayloadSenderConf()

	t.Run("one", func(t *testing.T) {
		e := &datadogEndpoint{host: "host1", apiKey: "key1"}
		sender, ok := newMultiSender([]endpoint{e}, cfg).(*queuableSender)
		assert := assert.New(t)
		assert.True(ok)
		assert.EqualValues(e, sender.endpoint)
		assert.EqualValues(cfg, sender.conf)
	})

	t.Run("multi", func(t *testing.T) {
		endpoints := []endpoint{
			&datadogEndpoint{host: "host1", apiKey: "key1"},
			&datadogEndpoint{host: "host2", apiKey: "key2"},
			&datadogEndpoint{host: "host3", apiKey: "key3"},
		}
		sender, ok := newMultiSender(endpoints, cfg).(*multiSender)
		assert := assert.New(t)
		assert.True(ok)
		assert.Len(sender.senders, 3)
		assert.Equal(3, cap(sender.mch))
		for i := range endpoints {
			s, ok := sender.senders[i].(*queuableSender)
			assert.True(ok)
			assert.EqualValues(endpoints[i], s.endpoint)
			assert.EqualValues(cfg, s.conf)
		}
	})
}

func TestMultiSender(t *testing.T) {
	t.Run("Start", func(t *testing.T) {
		mock1 := newMockSender()
		mock2 := newMockSender()
		multi := &multiSender{senders: []payloadSender{mock1, mock2}, mch: make(chan monitorEvent)}
		multi.Start()
		defer multi.Stop()

		assert := assert.New(t)
		assert.Equal(1, mock1.StartCalls())
		assert.Equal(1, mock2.StartCalls())
	})

	t.Run("Stop", func(t *testing.T) {
		mock1 := newMockSender()
		mock2 := newMockSender()
		multi := &multiSender{senders: []payloadSender{mock1, mock2}, mch: make(chan monitorEvent)}
		multi.Stop()

		assert := assert.New(t)
		assert.Equal(1, mock1.StopCalls())
		assert.Equal(1, mock2.StopCalls())

		select {
		case <-multi.mch:
		default:
			t.Fatal("monitor channel should be closed")
		}
	})

	t.Run("Send", func(t *testing.T) {
		mock1 := newMockSender()
		mock2 := newMockSender()
		p := &payload{creationDate: time.Now(), bytes: []byte{1, 2, 3}}
		multi := &multiSender{senders: []payloadSender{mock1, mock2}, mch: make(chan monitorEvent)}
		multi.Send(p)

		assert := assert.New(t)
		assert.Equal(p, mock1.SendCalls()[0])
		assert.Equal(p, mock2.SendCalls()[0])
	})

	t.Run("funnel", func(t *testing.T) {
		mock1 := newMockSender()
		mock2 := newMockSender()
		multi := &multiSender{senders: []payloadSender{mock1, mock2}, mch: make(chan monitorEvent)}
		multi.Start()
		defer multi.Stop()

		event1 := monitorEvent{typ: eventTypeSuccess, stats: sendStats{host: "ABC"}}
		event2 := monitorEvent{typ: eventTypeFailure, stats: sendStats{host: "QWE"}}

		mock1.monitorCh <- event1
		mock2.monitorCh <- event2

		assert.ElementsMatch(t,
			[]monitorEvent{event1, event2},
			[]monitorEvent{<-multi.mch, <-multi.mch},
		)
	})
}

func TestMockPayloadSender(t *testing.T) {
	p := &payload{creationDate: time.Now(), bytes: []byte{1, 2, 3}}
	mock := newMockSender()
	mock.Start()
	mock.Start()
	mock.Start()
	mock.Send(p)
	mock.Send(p)
	mock.Stop()

	assert := assert.New(t)
	assert.Equal(3, mock.StartCalls())
	assert.Equal(p, mock.SendCalls()[0])
	assert.Equal(p, mock.SendCalls()[1])
	assert.Equal(1, mock.StopCalls())

	mock.Reset()
	assert.Equal(0, mock.StartCalls())
	assert.Equal(0, mock.StopCalls())
	assert.Len(mock.SendCalls(), 0)
}

var _ payloadSender = (*mockPayloadSender)(nil)

type mockPayloadSender struct {
	startCalls uint64
	stopCalls  uint64

	mu        sync.Mutex
	sendCalls []*payload
	monitorCh chan monitorEvent
}

func newMockSender() *mockPayloadSender {
	return &mockPayloadSender{monitorCh: make(chan monitorEvent)}
}

func (m *mockPayloadSender) Reset() {
	atomic.SwapUint64(&m.startCalls, 0)
	atomic.SwapUint64(&m.stopCalls, 0)
	m.mu.Lock()
	m.sendCalls = m.sendCalls[:0]
	m.monitorCh = make(chan monitorEvent)
	m.mu.Unlock()
}

func (m *mockPayloadSender) Start() {
	atomic.AddUint64(&m.startCalls, 1)
}

func (m *mockPayloadSender) StartCalls() int {
	return int(atomic.LoadUint64(&m.startCalls))
}

// Stop must be called only once. It closes the monitor channel.
func (m *mockPayloadSender) Stop() {
	atomic.AddUint64(&m.stopCalls, 1)
	close(m.monitorCh)
}

func (m *mockPayloadSender) StopCalls() int {
	return int(atomic.LoadUint64(&m.stopCalls))
}

func (m *mockPayloadSender) Send(p *payload) {
	m.mu.Lock()
	m.sendCalls = append(m.sendCalls, p)
	m.mu.Unlock()
}

func (m *mockPayloadSender) SendCalls() []*payload {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sendCalls
}

func (m *mockPayloadSender) Monitor() <-chan monitorEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.monitorCh
}

func (m *mockPayloadSender) Run()                   {}
func (m *mockPayloadSender) setEndpoint(_ endpoint) {}
