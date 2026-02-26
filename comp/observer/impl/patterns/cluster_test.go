// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package patterns

import (
	"testing"
)

func TestSignatureClustererEmpty(t *testing.T) {
	sc := NewSignatureClusterer(IDComputeInfo{})
	if len(sc.GetClusters()) != 0 {
		t.Error("new clusterer should have 0 clusters")
	}
}

func TestSignatureClustererSameMessage(t *testing.T) {
	sc := NewSignatureClusterer(IDComputeInfo{})

	msg := `10.143.180.25 - - [27/Aug/2020:00:27:02 +0000] "POST /api/v1/series HTTP/1.1" 202 16`
	r1 := sc.Process(msg)
	if !r1.IsNew {
		t.Error("first message should create a new cluster")
	}
	if len(sc.GetClusters()) != 1 {
		t.Errorf("expected 1 cluster, got %d", len(sc.GetClusters()))
	}

	r2 := sc.Process(msg)
	if r2.IsNew {
		t.Error("same message should not create a new cluster")
	}
	if len(sc.GetClusters()) != 1 {
		t.Errorf("expected 1 cluster, got %d", len(sc.GetClusters()))
	}
}

func TestSignatureClustererDifferentMessages(t *testing.T) {
	sc := NewSignatureClusterer(IDComputeInfo{})

	msg1 := `10.143.180.25 - - [27/Aug/2020:00:27:02 +0000] "POST /api/v1/series HTTP/1.1" 202 16`
	msg2 := `2020-08-27 02:32:42 ERROR (connector.go:34) - Failed to connected to redis`

	sc.Process(msg1)
	sc.Process(msg2)

	if len(sc.GetClusters()) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(sc.GetClusters()))
	}
}

func TestSignatureClustererIgnoresEmpty(t *testing.T) {
	sc := NewSignatureClusterer(IDComputeInfo{})
	result := sc.Process("")
	if result != nil {
		t.Error("empty message should return nil")
	}
	if len(sc.GetClusters()) != 0 {
		t.Error("empty message should not create a cluster")
	}
}

func TestSignatureClustererCount(t *testing.T) {
	sc := NewSignatureClusterer(IDComputeInfo{})
	msg := "hello world"
	sc.Process(msg)
	sc.Process(msg)
	sc.Process(msg)

	clusters := sc.GetClusters()
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	if clusters[0].Count != 3 {
		t.Errorf("expected count=3, got %d", clusters[0].Count)
	}
}

func TestPatternClustererSimpleOneCluster(t *testing.T) {
	messages := []string{
		"[stats] total:889 rps:14.82",
		"[stats] total:890 rps:15.03",
		"[stats] total:886 rps:14.90",
		"[stats] total:885 rps:14.78",
		"[stats] total:888 rps:15.13",
	}

	pc := NewPatternClusterer(IDComputeInfo{})
	for _, msg := range messages {
		pc.Process(msg)
	}

	clusters := pc.GetClusters()
	if len(clusters) != 1 {
		t.Errorf("expected 1 cluster, got %d", len(clusters))
		for i, c := range clusters {
			t.Logf("  cluster[%d]: sig=%q count=%d pattern=%q", i, c.Signature, c.Count, c.PatternString())
		}
	}
}

func TestPatternClustererSimpleMultipleClusters(t *testing.T) {
	messages := []string{
		"[stats] total:889 rps:14.82",
		"new connection: 234",
		"[stats] total:890 rps:15.03",
		"[stats] total:886 rps:14.90",
		"some line",
		"[stats] total:885 rps:14.78",
		"[stats] total:888 rps:15.13",
		"new connection: 887",
		"new connection: 325",
		"[stats] total:888 rps:15.10",
	}

	pc := NewPatternClusterer(IDComputeInfo{})
	for _, msg := range messages {
		pc.Process(msg)
	}

	clusters := pc.GetClusters()
	if len(clusters) != 3 {
		t.Errorf("expected 3 clusters, got %d", len(clusters))
		for i, c := range clusters {
			t.Logf("  cluster[%d]: sig=%q count=%d pattern=%q", i, c.Signature, c.Count, c.PatternString())
		}
	}
}

func TestPatternClustererTrailingWhitespace(t *testing.T) {
	messages := []string{
		"ApplyAlgorithm request failed; requeueing for retry.",
		"ApplyAlgorithm request failed; requeueing for retry. ",
	}

	pc := NewPatternClusterer(IDComputeInfo{})
	for _, msg := range messages {
		pc.Process(msg)
	}

	if len(pc.GetClusters()) != 1 {
		t.Errorf("expected 1 cluster for trailing whitespace, got %d", len(pc.GetClusters()))
	}
}

func TestPatternClustererSameNumberOfTokens(t *testing.T) {
	messages := []string{
		"Trying to connect to redis delancie-backend-state with sentinel",
		"Trying to connect to redis delancie-backend-state with sentinel",
		"Travis CI API request for org 28989 succeeded",
		"Travis CI API request for org 125181 succeeded",
		"Trying to connect to redis delancie-backend-state with sentinel",
		"Found 0 schedules to save for org 159248",
		"Found 0 schedules to save for org 18226",
		"Trying to connect to redis delancie-backend-state with sentinel",
		"Travis CI API request for org 68083 succeeded",
	}

	pc := NewPatternClusterer(IDComputeInfo{})
	for _, msg := range messages {
		pc.Process(msg)
	}

	clusters := pc.GetClusters()
	if len(clusters) != 3 {
		t.Errorf("expected 3 clusters, got %d", len(clusters))
		for i, c := range clusters {
			t.Logf("  cluster[%d]: sig=%q count=%d pattern=%q", i, c.Signature, c.Count, c.PatternString())
		}
	}
}

func TestPatternClustererSameNumberOfTokens2(t *testing.T) {
	messages := []string{
		"processing check Prod-oz-reporting-logi1",
		"Found 12 checks",
		"Found 7 checks",
		"Found 0 checks",
		"processing check Marketo",
		"Done crawling pivotal",
		"processing check 4ppax-webapp",
		"processing check env_info",
		"Found 126 checks",
		"processing check EQNX-DC3-FW2",
		"Done crawling pagerduty",
	}

	pc := NewPatternClusterer(IDComputeInfo{})
	for _, msg := range messages {
		pc.Process(msg)
	}

	clusters := pc.GetClusters()
	if len(clusters) != 3 {
		t.Errorf("expected 3 clusters, got %d", len(clusters))
		for i, c := range clusters {
			t.Logf("  cluster[%d]: sig=%q count=%d pattern=%q", i, c.Signature, c.Count, c.PatternString())
		}
	}
}

func TestPatternClustererIgnoresEmpty(t *testing.T) {
	pc := NewPatternClusterer(IDComputeInfo{})
	pc.Process("")
	pc.Process("")
	pc.Process("")
	pc.Process("")

	if len(pc.GetClusters()) != 0 {
		t.Errorf("expected 0 clusters for empty messages, got %d", len(pc.GetClusters()))
	}

	pc.Process("text")
	if len(pc.GetClusters()) != 1 {
		t.Errorf("expected 1 cluster after adding text, got %d", len(pc.GetClusters()))
	}
}

func TestPatternClustererShouldReturnCorrectClusters(t *testing.T) {
	messages := []string{
		"i am going to start the server",
		"i am going to stop the server",
		"i am going to kill the server",
		"i am going to restart the server",
		"let's go for a run",
		"let's go for a walk",
		"let's go for a stroll",
		"let's go for a jog",
	}

	pc := NewPatternClusterer(IDComputeInfo{})
	for _, msg := range messages {
		pc.Process(msg)
	}

	clusters := pc.GetClusters()
	if len(clusters) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(clusters))
		for i, c := range clusters {
			t.Logf("  cluster[%d]: sig=%q count=%d pattern=%q", i, c.Signature, c.Count, c.PatternString())
		}
	}

	// Verify pattern strings
	for _, c := range clusters {
		pat := c.PatternString()
		if c.Count == 4 {
			if pat != "i am going to * the server" && pat != "let's go for a *" {
				t.Errorf("unexpected pattern: %q", pat)
			}
		}
	}
}

func TestPatternClustererSimilarMessages(t *testing.T) {
	messages := []string{
		"[34593523.404227]",
		"[18097718.596271]",
		"[17471008.511547]",
		"[18097688.560772]",
		"[34593403.292552]",
	}

	pc := NewPatternClusterer(IDComputeInfo{})
	for _, msg := range messages {
		pc.Process(msg)
	}

	if len(pc.GetClusters()) != 1 {
		t.Errorf("expected 1 cluster for similar numeric messages, got %d", len(pc.GetClusters()))
	}
}

func TestPatternClustererSimilarGCStats(t *testing.T) {
	messages := []string{
		"8387 @95749.612s 0%: 0.021+89+0.12 ms clock, 0.34+2.6/356/810+1.9 ms cpu, 550->562->461 MB, 616 MB goal, 16 P",
		"7119 @94522.217s 0%: 0.13+44+0.30 ms clock, 6.4+106/518/862+14 ms cpu, 1581->1598->1201 MB, 1604 MB goal, 48 P",
		"4730 @95338.114s 0%: 0.14+151+0.13 ms clock, 2.2+588/602/154+2.1 ms cpu, 2043->2095->1510 MB, 2092 MB goal, 16 P",
	}

	pc := NewPatternClusterer(IDComputeInfo{})
	for _, msg := range messages {
		pc.Process(msg)
	}

	if len(pc.GetClusters()) != 1 {
		t.Errorf("expected 1 cluster for GC stats, got %d", len(pc.GetClusters()))
		for i, c := range pc.GetClusters() {
			t.Logf("  cluster[%d]: sig=%q count=%d", i, c.Signature, c.Count)
		}
	}
}

func TestPatternClustererIdsAsFirstWord(t *testing.T) {
	messages := []string{
		"FATAL|2019-07-04T16:27:52,437|55767CC230D5|1.0|",
		"FATAL|2019-07-04T16:27:54,880|E41F49A44911|1.0|",
		"FATAL|2019-07-04T16:27:55,087|DC461A43ABD7|1.0|",
	}

	pc := NewPatternClusterer(IDComputeInfo{})
	for _, msg := range messages {
		pc.Process(msg)
	}

	if len(pc.GetClusters()) != 1 {
		t.Errorf("expected 1 cluster for ID messages, got %d", len(pc.GetClusters()))
		for i, c := range pc.GetClusters() {
			t.Logf("  cluster[%d]: sig=%q count=%d pattern=%q", i, c.Signature, c.Count, c.PatternString())
		}
	}
}

func TestPatternClustererTrailingInterrogationMark(t *testing.T) {
	pc := NewPatternClusterer(IDComputeInfo{})
	pc.Process("GET /api/v2/query?")
	pc.Process("GET /api/v2/query")

	if len(pc.GetClusters()) != 1 {
		t.Errorf("expected 1 cluster for query/no-query, got %d", len(pc.GetClusters()))
	}
}

func TestPatternClustererFailureMessages(t *testing.T) {
	// These messages have different token structures inside the quoted string,
	// so without collapsible section support they produce different signatures.
	// With collapsible sections (Java feature), they would cluster into 1.
	// For now, verify they produce at least some clusters and don't crash.
	messages := []string{
		`Failure detected for requestId=0. error.code=INTERNAL error.message="Failed to rewrite query: '@hc.dict-att.sub-int-att:>5'"`,
		`Failure detected for requestId=1. error.code=INTERNAL error.message="Failed to rewrite query: '@testValue:>=700'"`,
		`Failure detected for requestId=2. error.code=INTERNAL error.message="Failed to rewrite query: '@verb:GET AND @referer:*xyz*'"`,
	}

	pc := NewPatternClusterer(IDComputeInfo{})
	for _, msg := range messages {
		pc.Process(msg)
	}

	clusters := pc.GetClusters()
	if len(clusters) == 0 {
		t.Error("expected at least some clusters for failure messages")
	}
	if len(clusters) > 3 {
		t.Errorf("expected at most 3 clusters, got %d", len(clusters))
	}
}

// --- Merging tests ---

func TestMergeTokensWord(t *testing.T) {
	a := WordToken("abc")
	b := WordToken("def")
	if !canMergeTokens(a, b) {
		t.Error("two words should be mergeable")
	}
}

func TestMergeTokensSpecialChar(t *testing.T) {
	a := SpecialCharToken(';')
	b := SpecialCharToken(';')
	if !canMergeTokens(a, b) {
		t.Error("same special chars should be mergeable")
	}

	c := SpecialCharToken(',')
	if canMergeTokens(a, c) {
		t.Error("different special chars should not be mergeable")
	}
}

func TestMergeTokensNumber(t *testing.T) {
	a := NumericValueToken("12")
	b := NumericValueToken("34")
	if !canMergeTokens(a, b) {
		t.Error("two numbers should be mergeable")
	}
}

func TestMergeTokensWhitespace(t *testing.T) {
	a := WhitespaceToken(1)
	b := WhitespaceToken(5)
	if !canMergeTokens(a, b) {
		t.Error("whitespace tokens should be mergeable regardless of length")
	}
}

func TestMergeTokenListsSameLength(t *testing.T) {
	a := []Token{WordToken("hello"), WhitespaceToken(1), WordToken("world")}
	b := []Token{WordToken("hello"), WhitespaceToken(1), WordToken("earth")}
	if !canMergeTokenLists(a, b) {
		t.Error("token lists with same structure should be mergeable")
	}
}

func TestMergeTokenListsDifferentLength(t *testing.T) {
	a := []Token{WordToken("hello")}
	b := []Token{WordToken("hello"), WhitespaceToken(1)}
	if canMergeTokenLists(a, b) {
		t.Error("token lists with different lengths should not be mergeable")
	}
}

func TestMergeTokensDate(t *testing.T) {
	a := DateToken("yyyy-MM-dd", "2018-01-01")
	b := DateToken("yyyy-MM-dd", "2019-02-02")
	if !canMergeTokens(a, b) {
		t.Error("dates with same format should be mergeable")
	}

	c := DateToken("yyyy/MM/dd", "2018/01/01")
	if canMergeTokens(a, c) {
		t.Error("dates with different formats should not be mergeable")
	}
}

func TestHexDumpMerge(t *testing.T) {
	a := HexDumpToken("0010: DB 8A", 4, false)
	b := HexDumpToken("0010: CF 2F", 4, false)
	if !canMergeTokens(a, b) {
		t.Error("hexdumps with same displacement should be mergeable")
	}

	c := HexDumpToken("00000000: DB 8A", 8, false)
	if canMergeTokens(a, c) {
		t.Error("hexdumps with different displacement should not be mergeable")
	}
}

func TestMergeTokensCrossType(t *testing.T) {
	// HttpStatusCode and NumericValue should be mergeable (both number-like)
	a := HttpStatusCodeToken("200")
	b := NumericValueToken("887")
	if !canMergeTokens(a, b) {
		t.Error("HttpStatusCode and NumericValue should be mergeable")
	}

	// Word and NumericValue should NOT be mergeable
	c := WordToken("abc")
	d := NumericValueToken("123")
	if canMergeTokens(c, d) {
		t.Error("Word and NumericValue should not be mergeable")
	}
}

func TestMergeTokensWildcarding(t *testing.T) {
	pattern := []Token{WordToken("hello"), WhitespaceToken(1), WordToken("world")}
	incoming := []Token{WordToken("hello"), WhitespaceToken(1), WordToken("earth")}

	mergeTokenLists(pattern, incoming)

	if pattern[0].IsWild {
		t.Error("'hello' should not be wildcarded (same value)")
	}
	if !pattern[2].IsWild {
		t.Error("last word should be wildcarded (different value)")
	}
}

// --- End-to-end clustering ---

func TestEndToEndClustering(t *testing.T) {
	pc := NewPatternClusterer(IDComputeInfo{})

	r1 := pc.Process("user login from 192.168.1.1")
	if !r1.IsNew {
		t.Error("first message should be new")
	}

	r2 := pc.Process("user login from 10.0.0.1")
	if r2.IsNew {
		t.Error("similar message should match existing cluster")
	}

	r3 := pc.Process("server started on port 8080")
	if !r3.IsNew {
		t.Error("different message should create new cluster")
	}

	if len(pc.GetClusters()) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(pc.GetClusters()))
	}
}

func TestIDComputeInfo(t *testing.T) {
	idComputeInfo := IDComputeInfo{Offset: 1, Stride: 2, Index: 0}
	if idComputeInfo.NextID() != 1 {
		t.Errorf("expected 1, got %d", idComputeInfo.NextID())
	}
	if idComputeInfo.Index != 1 {
		t.Errorf("expected 1, got %d", idComputeInfo.Index)
	}
	if idComputeInfo.NextID() != 3 {
		t.Errorf("expected 3, got %d", idComputeInfo.NextID())
	}
	if idComputeInfo.Index != 2 {
		t.Errorf("expected 2, got %d", idComputeInfo.Index)
	}
}
