package util

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func getTestVersionName(i int) string {
	return fmt.Sprintf("Version %d", i)
}

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

func validateHistory(t *testing.T, i int, versionHistoryFilePath string) {
	file, err := ioutil.ReadFile(versionHistoryFilePath)
	assert.Nil(t, err)

	history := versionHistoryEntries{}

	err = json.Unmarshal(file, &history)
	assert.Nil(t, err)

	k := 0
	for j := max(0, i-maxVersionHistoryEntries+1); j <= i; j++ {
		// Old entries might have been erased.
		assert.Equal(t, getTestVersionName(j), history.Entries[k].Version)
		assert.NotEqual(t, time.Time{}, history.Entries[k].Timestamp)
		k++
	}
}

func TestVersionHistory(t *testing.T) {
	versionHistoryFilePath := "version-history.json"
	_ = os.Remove(versionHistoryFilePath)

	// Make sure we cover the file trimming case.
	tests := maxVersionHistoryEntries + 10

	for i := 0; i < tests; i++ {
		testVersion := getTestVersionName(i)

		// Write a new entry, the last 10 test will erase earlier entries.
		logVersionHistoryToFile(versionHistoryFilePath, testVersion, time.Now().UTC())
		// Read the file, validate the result.
		validateHistory(t, i, versionHistoryFilePath)

		// Write the same entry with a dummy timestamp. This should not replace any entry in the file.
		logVersionHistoryToFile(versionHistoryFilePath, testVersion, time.Time{})
		// Validate the result and make sure the dummy timestamp is not in any entry.
		validateHistory(t, i, versionHistoryFilePath)
	}

	_ = os.Remove(versionHistoryFilePath)
}
