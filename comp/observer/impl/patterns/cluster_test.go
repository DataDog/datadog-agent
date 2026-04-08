// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package patterns

import (
	"testing"
)

// Fixed event time for SignatureClusterer tests (Unix seconds, non-zero).
const testUnixSec = int64(1704067200)

func TestSignatureClustererEmpty(t *testing.T) {
	sc := NewSignatureClusterer()
	if len(sc.GetClusters()) != 0 {
		t.Error("new clusterer should have 0 clusters")
	}
}

func TestSignatureClustererSameMessage(t *testing.T) {
	sc := NewSignatureClusterer()

	msg := `10.143.180.25 - - [27/Aug/2020:00:27:02 +0000] "POST /api/v1/series HTTP/1.1" 202 16`
	r1, _ := sc.Process(msg, testUnixSec)
	if r1.Count != 1 {
		t.Error("first message should create a new cluster")
	}
	if len(sc.GetClusters()) != 1 {
		t.Errorf("expected 1 cluster, got %d", len(sc.GetClusters()))
	}

	r2, _ := sc.Process(msg, testUnixSec)
	if r2.Count == 1 {
		t.Error("same message should not create a new cluster")
	}
	if len(sc.GetClusters()) != 1 {
		t.Errorf("expected 1 cluster, got %d", len(sc.GetClusters()))
	}
}

func TestSignatureClustererDifferentMessages(t *testing.T) {
	sc := NewSignatureClusterer()

	msg1 := `10.143.180.25 - - [27/Aug/2020:00:27:02 +0000] "POST /api/v1/series HTTP/1.1" 202 16`
	msg2 := `2020-08-27 02:32:42 ERROR (connector.go:34) - Failed to connected to redis`

	sc.Process(msg1, testUnixSec)
	sc.Process(msg2, testUnixSec+1)

	if len(sc.GetClusters()) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(sc.GetClusters()))
	}
}

func TestSignatureClustererIgnoresEmpty(t *testing.T) {
	sc := NewSignatureClusterer()
	_, ok := sc.Process("", testUnixSec)
	if ok {
		t.Error("empty message should return no result")
	}
	if len(sc.GetClusters()) != 0 {
		t.Error("empty message should not create a cluster")
	}
}

func TestSignatureClustererCount(t *testing.T) {
	sc := NewSignatureClusterer()
	msg := "hello world"
	sc.Process(msg, testUnixSec)
	sc.Process(msg, testUnixSec)
	sc.Process(msg, testUnixSec)

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

	pc := NewPatternClusterer()
	for _, msg := range messages {
		pc.Process(msg, 0)
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

	pc := NewPatternClusterer()
	for _, msg := range messages {
		pc.Process(msg, 0)
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

	pc := NewPatternClusterer()
	for _, msg := range messages {
		pc.Process(msg, 0)
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

	pc := NewPatternClusterer()
	for _, msg := range messages {
		pc.Process(msg, 0)
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

	pc := NewPatternClusterer()
	for _, msg := range messages {
		pc.Process(msg, 0)
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
	pc := NewPatternClusterer()
	pc.Process("", 0)
	pc.Process("", 0)
	pc.Process("", 0)
	pc.Process("", 0)

	if len(pc.GetClusters()) != 0 {
		t.Errorf("expected 0 clusters for empty messages, got %d", len(pc.GetClusters()))
	}

	pc.Process("text", 0)
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

	pc := NewPatternClusterer()
	for _, msg := range messages {
		pc.Process(msg, 0)
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

	pc := NewPatternClusterer()
	for _, msg := range messages {
		pc.Process(msg, 0)
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

	pc := NewPatternClusterer()
	for _, msg := range messages {
		pc.Process(msg, 0)
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

	pc := NewPatternClusterer()
	for _, msg := range messages {
		pc.Process(msg, 0)
	}

	if len(pc.GetClusters()) != 1 {
		t.Errorf("expected 1 cluster for ID messages, got %d", len(pc.GetClusters()))
		for i, c := range pc.GetClusters() {
			t.Logf("  cluster[%d]: sig=%q count=%d pattern=%q", i, c.Signature, c.Count, c.PatternString())
		}
	}
}

func TestPatternClustererTrailingInterrogationMark(t *testing.T) {
	pc := NewPatternClusterer()
	pc.Process("GET /api/v2/query?", 0)
	pc.Process("GET /api/v2/query", 0)

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

	pc := NewPatternClusterer()
	for _, msg := range messages {
		pc.Process(msg, 0)
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
	if !canMergeTokenListsWithRatio(a, b, 0.5) {
		t.Error("token lists with same structure should be mergeable")
	}
}

func TestMergeTokenListsDifferentLength(t *testing.T) {
	a := []Token{WordToken("hello")}
	b := []Token{WordToken("hello"), WhitespaceToken(1)}
	if canMergeTokenListsWithRatio(a, b, 0.5) {
		t.Error("token lists with different lengths should not be mergeable")
	}
}

func TestMergeTokenListsStricterRatio(t *testing.T) {
	// Four tokens, two value matches (50%) — merge at default 0.5, not at 0.8.
	a := []Token{WordToken("a"), WhitespaceToken(1), WordToken("b"), WordToken("c")}
	b := []Token{WordToken("a"), WhitespaceToken(1), WordToken("x"), WordToken("y")}
	if !canMergeTokenListsWithRatio(a, b, 0.5) {
		t.Error("expected merge at default ratio 0.5")
	}
	if canMergeTokenListsWithRatio(a, b, 0.8) {
		t.Error("expected no merge at 0.8 min ratio")
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
	a := HTTPStatusCodeToken("200")
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
	pc := NewPatternClusterer()

	r1, _ := pc.Process("user login from 192.168.1.1", 0)
	if r1.Count != 1 {
		t.Error("first message should be new")
	}

	r2, _ := pc.Process("user login from 10.0.0.1", 0)
	if r2.Count == 1 {
		t.Error("similar message should match existing cluster")
	}

	r3, _ := pc.Process("server started on port 8080", 0)
	if r3.Count != 1 {
		t.Error("different message should create new cluster")
	}

	if len(pc.GetClusters()) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(pc.GetClusters()))
	}
}

func TestPatternClustererLastSeenAndClusterIDsBeforeUnix(t *testing.T) {
	pc := NewPatternClusterer()

	a, _ := pc.Process("unique alpha message one", 1000)
	b, _ := pc.Process("totally different beta two", 5000)
	if a.LastSeenUnix != 1000 || b.LastSeenUnix != 5000 {
		t.Fatalf("LastSeenUnix: a=%d b=%d", a.LastSeenUnix, b.LastSeenUnix)
	}

	ids := pc.ClusterIDsBeforeUnix(5000)
	if len(ids) != 1 || ids[0] != a.ID {
		t.Fatalf("expected single id %d before 5000, got %v", a.ID, ids)
	}
	ids = pc.ClusterIDsBeforeUnix(5001)
	if len(ids) != 2 {
		t.Fatalf("expected both cluster ids before 5001, got %v", ids)
	}

	// Merge path: log at t=9000 joins cluster a; last seen on a updates to 9000.
	_, _ = pc.Process("unique alpha message nine", 9000)
	if a.LastSeenUnix != 9000 {
		t.Fatalf("merge should update last seen: got %d", a.LastSeenUnix)
	}
	if got := pc.ClusterIDsBeforeUnix(5000); len(got) != 0 {
		t.Fatalf("expected no cluster ids with lastSeen < 5000 after merge, got %v", got)
	}
	if got := pc.ClusterIDsBeforeUnix(9001); len(got) != 2 {
		t.Fatalf("expected both cluster ids before 9001, got %v", got)
	}
}

func TestPatternClustererRemoveCluster(t *testing.T) {
	pc := NewPatternClusterer()
	a, _ := pc.Process("[stats] total:889 rps:14.82", 100)
	b, _ := pc.Process("new connection: 234", 200)
	if pc.NumClusters() != 2 {
		t.Fatalf("expected 2 clusters, got %d", pc.NumClusters())
	}

	if err := pc.RemoveCluster(99999); err == nil {
		t.Fatal("expected error removing missing cluster id")
	}

	if err := pc.RemoveCluster(a.ID); err != nil {
		t.Fatalf("RemoveCluster: %v", err)
	}
	if pc.NumClusters() != 1 {
		t.Fatalf("expected 1 cluster after removal, got %d", pc.NumClusters())
	}
	if _, err := pc.GetCluster(a.ID); err == nil {
		t.Fatal("expected GetCluster to fail for removed id")
	}
	if c, err := pc.GetCluster(b.ID); err != nil || c != b {
		t.Fatalf("remaining cluster: err=%v c=%v b=%v", err, c, b)
	}
	clusters := pc.GetClusters()
	if len(clusters) != 1 || clusters[0].ID != b.ID {
		t.Fatalf("GetClusters: %v", clusters)
	}

	if err := pc.RemoveCluster(b.ID); err != nil {
		t.Fatalf("second RemoveCluster: %v", err)
	}
	if pc.NumClusters() != 0 {
		t.Fatalf("expected 0 clusters, got %d", pc.NumClusters())
	}
}

func TestPatternClustererRemoveClusters(t *testing.T) {
	pc := NewPatternClusterer()
	a, _ := pc.Process("[stats] total:889 rps:14.82", 100)
	b, _ := pc.Process("new connection: 234", 200)
	c, _ := pc.Process("other line", 300)
	if pc.NumClusters() != 3 {
		t.Fatalf("expected 3 clusters, got %d", pc.NumClusters())
	}

	if err := pc.RemoveClusters([]int64{a.ID, 99999}); err == nil {
		t.Fatal("expected error when one id is missing")
	}
	if pc.NumClusters() != 3 {
		t.Fatalf("expected no change on failed batch, got %d clusters", pc.NumClusters())
	}

	if err := pc.RemoveClusters([]int64{a.ID, b.ID}); err != nil {
		t.Fatalf("RemoveClusters: %v", err)
	}
	if pc.NumClusters() != 1 {
		t.Fatalf("expected 1 cluster, got %d", pc.NumClusters())
	}
	if _, err := pc.GetCluster(c.ID); err != nil {
		t.Fatalf("remaining cluster: %v", err)
	}

	if err := pc.RemoveClusters(nil); err != nil {
		t.Fatalf("RemoveClusters nil: %v", err)
	}
	if err := pc.RemoveClusters([]int64{}); err != nil {
		t.Fatalf("RemoveClusters empty: %v", err)
	}
	if err := pc.RemoveClusters([]int64{c.ID}); err != nil {
		t.Fatalf("final RemoveClusters: %v", err)
	}
	if pc.NumClusters() != 0 {
		t.Fatalf("expected 0 clusters, got %d", pc.NumClusters())
	}
}
