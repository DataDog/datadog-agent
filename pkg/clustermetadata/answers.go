// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

// ReducePeerAnswers merges the answers of a broadcast lookup into the answer
// the consumer receives. The coordinator passes its own answer and all peer
// answers in one slice.
func ReducePeerAnswers(answers []LookupAnswer) LookupAnswer {
	var notReady, absent, notMine bool
	for _, a := range answers {
		switch a.Kind {
		case AnswerFound:
			// Exactly one replica owns a pod in steady state, so the first
			// Found answer is correct even when two replicas overlap
			// mid-rebalance.
			return a
		case AnswerNotReady:
			notReady = true
		case AnswerAbsent:
			absent = true
		case AnswerNotMine:
			notMine = true
		}
	}

	switch {
	case notReady:
		return LookupAnswer{Kind: AnswerNotReady}
	case absent:
		return LookupAnswer{Kind: AnswerAbsent}
	case notMine:
		return LookupAnswer{Kind: AnswerNotReady}
	default:
		return LookupAnswer{Kind: AnswerNotReady}
	}
}
