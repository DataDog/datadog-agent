// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package sampler

import (
	"hash/fnv"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

// Signature is a hash representation of trace or a service, used to identify
// simlar signatures.
type Signature uint64

// spanHash is the type of the hashes used during the computation of a signature
// Use FNV for hashing since it is super-cheap and we have no cryptographic needs
type spanHash uint32
type spanHashSlice []spanHash

func (p spanHashSlice) Len() int           { return len(p) }
func (p spanHashSlice) Less(i, j int) bool { return p[i] < p[j] }
func (p spanHashSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func sortHashes(hashes []spanHash)         { sort.Sort(spanHashSlice(hashes)) }

// computeSignatureWithRootAndEnv generates the signature of a trace knowing its root
// Signature based on the hash of (env, service, name, resource, is_error) for the root, plus the set of
// (env, service, name, is_error) of each span.
func computeSignatureWithRootAndEnv(trace pb.Trace, root *pb.Span, env string) Signature {
	rootHash := computeRootHash(*root, env)
	spanHashes := make([]spanHash, 0, len(trace))

	for i := range trace {
		spanHashes = append(spanHashes, computeSpanHash(trace[i], env))
	}

	// Now sort, dedupe then merge all the hashes to build the signature
	sortHashes(spanHashes)

	last := spanHashes[0]
	traceHash := last ^ rootHash
	for i := 1; i < len(spanHashes); i++ {
		if spanHashes[i] != last {
			last = spanHashes[i]
			traceHash = spanHashes[i] ^ traceHash
		}
	}

	return Signature(traceHash)
}

// ServiceSignature represents a unique way to identify a service.
type ServiceSignature struct{ Name, Env string }

// Hash generates the signature of a trace with minimal information such as
// service and env, this is typically used by distributed sampling based on
// priority, and used as a key to store the desired rate for a given
// service,env tuple.
func (s ServiceSignature) Hash() Signature {
	h := fnv.New32a()
	h.Write([]byte(s.Name))
	h.Write([]byte{','})
	h.Write([]byte(s.Env))
	return Signature(h.Sum32())
}

func (s ServiceSignature) String() string {
	return "service:" + s.Name + ",env:" + s.Env
}

func computeSpanHash(span *pb.Span, env string) spanHash {
	h := fnv.New32a()
	h.Write([]byte(env))
	h.Write([]byte(span.Service))
	h.Write([]byte(span.Name))
	h.Write([]byte{byte(span.Error)})

	return spanHash(h.Sum32())
}

func computeRootHash(span pb.Span, env string) spanHash {
	h := fnv.New32a()
	h.Write([]byte(env))
	h.Write([]byte(span.Service))
	h.Write([]byte(span.Name))
	h.Write([]byte(span.Resource))
	h.Write([]byte{byte(span.Error)})

	return spanHash(h.Sum32())
}
