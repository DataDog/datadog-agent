package appsec

import (
	"encoding/json"
	"sync"
	"time"

	waf "github.com/DataDog/go-libddwaf"

	rc "github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
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
		Env:          key.env,
		ServiceName:  key.serviceName,
		PollRate:     time.Second * 5,
		Products:     []string{"ASM_DATA"},
		RuntimeID:    "test",
		Capabilities: []byte{'B', 'A', '=', '='},
	})
	if err != nil {
		log.Errorf("appsec: couldn't init the rc client: %v", err)
		return
	}
	update := func(u ProductUpdate) error {
		if len(u) > 0 {
			return m.createWafContextForService(key, u)
		}
		return nil
	}
	client.RegisterCallback(update, "ASM_DATA")
	go client.Start()
	m.lock.Lock()
	m.rcClients[key] = client
	m.lock.Unlock()
}

// mergeRulesDataEntries merges two slices of rules data entries together, removing duplicates and
// only keeping the longest expiration values for similar entries.
func mergeRulesDataEntries(entries1, entries2 []rc.ASMDataRuleDataEntry) []rc.ASMDataRuleDataEntry {
	mergeMap := map[string]int64{}

	for _, entry := range entries1 {
		mergeMap[entry.Value] = entry.Expiration
	}
	// Replace the entry only if the new expiration timestamp goes later than the current one
	// If no expiration timestamp was provided (default to 0), then the data doesn't expire
	for _, entry := range entries2 {
		if exp, ok := mergeMap[entry.Value]; !ok || entry.Expiration == 0 || entry.Expiration > exp {
			mergeMap[entry.Value] = entry.Expiration
		}
	}
	// Create the final slice and return it
	entries := make([]rc.ASMDataRuleDataEntry, 0, len(mergeMap))
	for val, exp := range mergeMap {
		entries = append(entries, rc.ASMDataRuleDataEntry{
			Value:      val,
			Expiration: exp,
		})
	}
	return entries
}

func (m *Manager) createWafContextForService(key serviceKey, update ProductUpdate) error {
	handle, err := NewWAFHandle()
	if err != nil {
		return err
	}

	allRulesData := make(map[string]map[string]rc.ASMDataRuleData)

	for path, raw := range update {
		log.Debugf("appsec: Remote config: processing %s", path)
		var rulesData rc.ASMDataRulesData
		if err := json.Unmarshal(raw, &rulesData); err != nil {
			log.Debugf("appsec: Remote config: error while unmarshalling payload for %s: %v. Configuration won't be applied.", path, err)
			continue
		}

		// Check each entry against allRulesData to see if merging is necessary
		for _, ruleData := range rulesData.RulesData {
			if allRulesData[ruleData.ID] == nil {
				allRulesData[ruleData.ID] = make(map[string]rc.ASMDataRuleData)
			}
			if data, ok := allRulesData[ruleData.ID][ruleData.Type]; ok {
				// Merge rules data entries with the same ID and Type
				data.Data = mergeRulesDataEntries(data.Data, ruleData.Data)
				allRulesData[ruleData.ID][ruleData.Type] = data
			} else {
				allRulesData[ruleData.ID][ruleData.Type] = ruleData
			}
		}
	}

	// Aggregate all the rules data before passing it over to the WAF
	var rulesData []rc.ASMDataRuleData
	for _, m := range allRulesData {
		for _, data := range m {
			rulesData = append(rulesData, data)
		}
	}

	buf, err := json.Marshal(rulesData)
	if err != nil {
		return err
	}

	if err := handle.UpdateRuleData(buf); err != nil {
		log.Debugf("appsec: Remote config: could not update WAF rule data: %v.", err)
		return err
	}

	m.lock.Lock()
	m.wafHandles[key] = handle
	m.lock.Unlock()
	return nil
}

func (m *Manager) GetWafContextForService(serviceName, env string) *waf.Context {
	needsRcClient := false
	key := serviceKey{serviceName, env}
	m.lock.RLock()
	handle, ok := m.wafHandles[key]
	if !ok {
		if _, ok = m.rcClients[key]; !ok {
			needsRcClient = true
		}
	}
	m.lock.RUnlock()
	if needsRcClient {
		m.subscribeWafForService(key)
	}
	if handle == nil {
		handle = m.defaultWafHandle
	}
	return waf.NewContext(handle)
}
