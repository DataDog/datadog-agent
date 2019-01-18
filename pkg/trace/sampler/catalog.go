package sampler

import "sync"

const defaultServiceRateKey = "service:,env:"

// serviceKeyCatalog reverse-maps service signatures to their generated hashes for
// easy look up.
type serviceKeyCatalog struct {
	mu     sync.Mutex
	lookup map[ServiceSignature]Signature
}

// newServiceLookup returns a new serviceKeyCatalog.
func newServiceLookup() *serviceKeyCatalog {
	return &serviceKeyCatalog{
		lookup: make(map[ServiceSignature]Signature),
	}
}

func (cat *serviceKeyCatalog) register(svcSig ServiceSignature) Signature {
	hash := svcSig.Hash()
	cat.mu.Lock()
	cat.lookup[svcSig] = hash
	cat.mu.Unlock()
	return hash
}

// ratesByService returns a map of service signatures mapping to the rates identified using
// the signatures.
func (cat serviceKeyCatalog) ratesByService(rates map[Signature]float64, totalScore float64) map[ServiceSignature]float64 {
	rbs := make(map[ServiceSignature]float64, len(rates)+1)
	defer cat.mu.Unlock()
	cat.mu.Lock()
	for key, sig := range cat.lookup {
		if rate, ok := rates[sig]; ok {
			rbs[key] = rate
		} else {
			delete(cat.lookup, key)
		}
	}
	rbs[ServiceSignature{}] = totalScore
	return rbs
}
