package appsec

import (
	"sync"
	"time"

	waf "github.com/DataDog/go-libddwaf"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

type serviceKey struct {
	serviceName string
	env         string
}

type Manager struct {
	defaultWafHandle *waf.Handle
	wafHandles       map[serviceKey]*waf.Handle
	rcClients        map[serviceKey]*Client
	lock             sync.RWMutex
}

func NewManager(defaultWafHandle *waf.Handle) *Manager {
	return &Manager{
		defaultWafHandle: defaultWafHandle,
		wafHandles:       make(map[serviceKey]*waf.Handle),
		rcClients:        make(map[serviceKey]*Client),
	}
}

func (m *Manager) subscribeWafForService(key serviceKey) {
	client, err := NewClient(ClientConfig{
		Env:           key.env,
		ServiceName:   key.serviceName,
		PollRate:      time.Second * 5,
		Products:      []string{"ASM_DATA"},
		RuntimeID:     "test",
		Capabilities:  []byte{'B', 'A', '=', '='},
	})
	if err != nil {
		log.Errorf("couldn't init the rc client: %v", err)
		return
	}
	update := func(update ProductUpdate) error {
		log.Infof("GOT UPDATE")
		return nil
	}
	client.RegisterCallback(update, "ASM_DATA")
	go client.Start()
	m.lock.Lock()
	m.rcClients[key] = client
	m.lock.Unlock()
}

func (m *Manager) GetWafContextForService(serviceName, env string) *waf.Context {
	key := serviceKey{serviceName, env}
	m.lock.RLock()
	handle, ok := m.wafHandles[key]
	m.lock.RUnlock()
	if !ok {
		m.subscribeWafForService(key)
		handle = m.defaultWafHandle
	}
	return waf.NewContext(handle)
}
