package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/ebpf/verifier"
)

func main() {
	var objectFiles []string
	var err error
	if err := filepath.WalkDir("../../bytecode/build/co-re", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		if strings.Contains(path, "-debug") || !strings.HasSuffix(path, ".o") {
			return nil
		}
		objectFiles = append(objectFiles, path)

		return nil
	}); err != nil {
		log.Fatalf("failed to discoved all object files: %v", err)
	}
	stats, err := verifier.BuildVerifierStats(objectFiles)
	if err != nil {
		log.Fatalf("failed to build verifier stats: %v", err)
	}

	j, _ := json.Marshal(stats)
	fmt.Println(string(j))
}
