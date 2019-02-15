package two_check

import "testing"

func TestGetCheckAgent(t *testing.T) {
	check := getFakeAgentCheck()

	if check == nil {
		t.Fatal("Agent not found")
	}
}

func TestRunCheckAgent(t *testing.T) {
	res := runFakeAgentCheck()

	if res == "" {
		t.Fatal("Run failed")
	}
}
