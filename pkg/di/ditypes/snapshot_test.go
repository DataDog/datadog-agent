package ditypes

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDynamicInstrumentationLogJSONRoundTrip(t *testing.T) {
	files := []string{
		"testdata/snapshot-00.json",
		"testdata/snapshot-01.json",
		"testdata/snapshot-02.json",
	}
	for _, filePath := range files {
		file, err := os.Open(filePath)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()

		bytes, err := io.ReadAll(file)
		if err != nil {
			log.Fatal(err)
		}

		var s SnapshotUpload
		err = json.Unmarshal(bytes, &s)
		if err != nil {
			log.Fatal(err)
		}

		mBytes, err := json.Marshal(s)
		if err != nil {
			log.Fatal(err)
		}

		assert.JSONEq(t, string(bytes), string(mBytes))
	}
}
