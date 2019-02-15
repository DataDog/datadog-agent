package two_check

import "testing"

func TestGetCheckAgent(t *testing.T) {
	check := getFakeCheck()

	if check == nil {
		t.Fatal("Check not found")
	}
}

func TestRunCheckAgent(t *testing.T) {
	res := runFakeCheck()

	if res == "" {
		t.Fatal("Run failed")
	}
}
