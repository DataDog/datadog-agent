// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.n

package processor

import (
	"encoding/csv"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

type regexBenchCase struct {
	Name               string
	Pattern            string
	Type               string
	ReplacePlaceholder string
	GptOptimized       string
	MistralOptimized   string
	QwenOptimized      string
	Match              string
	Miss               string
}

func sanitizeForASCII(s string) string {
    // Normalize common Unicode quote/space variants to ASCII
    replacer := strings.NewReplacer(
        "\u00A0", " ",    // NBSP → space
        "“", "\"", "”", "\"", // curly double quotes → ASCII "
        "‘", "'", "’", "'",   // curly single quotes → ASCII '
        "\u200B", "",     // zero width space
        "\u200C", "",
        "\u200D", "",
        "\uFEFF", "",     // BOM
    )
    s = replacer.Replace(s)

    // Strip any remaining non-ASCII characters
    cleaned := make([]rune, 0, len(s))
    for _, r := range s {
        if r <= 127 {
            cleaned = append(cleaned, r)
        }
    }
    return string(cleaned)
}


// loadRegexBenchCases loads patterns from CSV files
func loadRegexBenchCases() []regexBenchCase {
	// Load base patterns and test strings from control_regex.csv
	const controlCSV = "testdata/control_regex.csv"
	const optimizedCSV = "testdata/final_optimized_regex.csv"

	// First, load control_regex.csv for base patterns and test strings
	file, err := os.Open(controlCSV)
	if err != nil {
		return nil
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1
	controlRecords, err := reader.ReadAll()
	if err != nil || len(controlRecords) <= 1 {
		return nil
	}

	// Parse control CSV (columns: name, type, pattern, replace_placeholder, _, match, miss)
	casesMap := make(map[string]*regexBenchCase)
	for _, record := range controlRecords[1:] {
		if len(record) != 7 {
			continue
		}
		for i := range record {
			record[i] = sanitizeForASCII(record[i])
		}

		casesMap[record[0]] = &regexBenchCase{
			Name:               record[0],
			Type:               record[1],
			Pattern:            record[2],
			ReplacePlaceholder: record[3],
			Match:              record[5],
			Miss:               record[6],
		}
	}

	// Now load optimized patterns from final_optimized_regex.csv
	file2, err := os.Open(optimizedCSV)
	if err == nil {
		defer file2.Close()

		reader2 := csv.NewReader(file2)
		reader2.LazyQuotes = true
		reader2.FieldsPerRecord = -1
		optimizedRecords, err := reader2.ReadAll()

		if err == nil && len(optimizedRecords) > 1 {
			// Parse optimized CSV (columns: name, type, pattern, replace_placeholder, gpt_new_regex, mistral_new_regex, qwen_new_regex)
			for _, record := range optimizedRecords[1:] {
				if len(record) < 7 {
					continue
				}
				for i := range record {
					record[i] = sanitizeForASCII(record[i])
				}

				name := record[0]
				if benchCase, exists := casesMap[name]; exists {
					benchCase.GptOptimized = record[4]
					benchCase.MistralOptimized = record[5]
					benchCase.QwenOptimized = record[6]
				}
			}
		}
	}

	// Convert map to slice
	var cases []regexBenchCase
	for _, c := range casesMap {
		cases = append(cases, *c)
	}

	return cases
}

func BenchmarkRegexPatterns(b *testing.B) {
	cases := loadRegexBenchCases()

	// Validate and filter out invalid patterns
	validCases := []regexBenchCase{}
	skippedCount := 0

	// Counters for each optimized pattern variant
	gptMatchMismatchCount := 0
	gptMissMismatchCount := 0
	mistralMatchMismatchCount := 0
	mistralMissMismatchCount := 0
	qwenMatchMismatchCount := 0
	qwenMissMismatchCount := 0

	fmt.Printf("\n=== Validating %d regex patterns ===\n", len(cases))
	for i, rc := range cases {
		// Try to compile the regex to check if it's valid
		re, err := regexp.Compile(rc.Pattern)
		if err != nil {
			fmt.Printf("[%d] SKIPPED %s: invalid regex - %v\n", i+1, rc.Name, err)
			skippedCount++
			continue
		}

		fmt.Printf("[%d] %s (type: %s)\n    Pattern: %s\n", i+1, rc.Name, rc.Type, rc.Pattern)
		if rc.ReplacePlaceholder != "" {
			fmt.Printf("    Placeholder: %s\n", rc.ReplacePlaceholder)
		}

		// Validate optimized patterns if present
		var gptRe, mistralRe, qwenRe *regexp.Regexp
		if rc.GptOptimized != "" {
			gptRe, err = regexp.Compile(rc.GptOptimized)
			if err != nil {
				fmt.Printf("    ⚠️  Invalid GPT optimized regex - %v\n", err)
			} else {
				fmt.Printf("    GPT Optimized: %s\n", rc.GptOptimized)
			}
		}
		if rc.MistralOptimized != "" {
			mistralRe, err = regexp.Compile(rc.MistralOptimized)
			if err != nil {
				fmt.Printf("    ⚠️  Invalid Mistral optimized regex - %v\n", err)
			} else {
				fmt.Printf("    Mistral Optimized: %s\n", rc.MistralOptimized)
			}
		}
		if rc.QwenOptimized != "" {
			qwenRe, err = regexp.Compile(rc.QwenOptimized)
			if err != nil {
				fmt.Printf("    ⚠️  Invalid Qwen optimized regex - %v\n", err)
			} else {
				fmt.Printf("    Qwen Optimized: %s\n", rc.QwenOptimized)
			}
		}

		// Validate match and miss test strings against original pattern ONLY
		// If original validation fails, skip both original and optimized benchmarks
		originalValidationFailed := false
		if rc.Match != "" {
			if !re.MatchString(rc.Match) {
				fmt.Printf("    ⚠️  ORIGINAL VALIDATION FAILED: 'match' string should match but doesn't\n")
				fmt.Printf("        Match string: %s\n", rc.Match)
				originalValidationFailed = true
			}
		}
		if rc.Miss != "" {
			if re.MatchString(rc.Miss) {
				fmt.Printf("    ⚠️  ORIGINAL VALIDATION FAILED: 'miss' string should NOT match but does\n")
				fmt.Printf("        Miss string: %s\n", rc.Miss)
				originalValidationFailed = true
			}
		}

		if originalValidationFailed {
			fmt.Printf("    SKIPPED due to original pattern validation failures\n")
			skippedCount++
			continue
		}

		// Check optimized pattern validations but only warn, don't skip
		if gptRe != nil {
			if rc.Match != "" && !gptRe.MatchString(rc.Match) {
				fmt.Printf("    ⚠️  GPT: doesn't match 'match' string\n")
				gptMatchMismatchCount++
			}
			if rc.Miss != "" && gptRe.MatchString(rc.Miss) {
				fmt.Printf("    ⚠️  GPT: matches 'miss' string (should not)\n")
				gptMissMismatchCount++
			}
		}
		if mistralRe != nil {
			if rc.Match != "" && !mistralRe.MatchString(rc.Match) {
				fmt.Printf("    ⚠️  Mistral: doesn't match 'match' string\n")
				mistralMatchMismatchCount++
			}
			if rc.Miss != "" && mistralRe.MatchString(rc.Miss) {
				fmt.Printf("    ⚠️  Mistral: matches 'miss' string (should not)\n")
				mistralMissMismatchCount++
			}
		}
		if qwenRe != nil {
			if rc.Match != "" && !qwenRe.MatchString(rc.Match) {
				fmt.Printf("    ⚠️  Qwen: doesn't match 'match' string\n")
				qwenMatchMismatchCount++
			}
			if rc.Miss != "" && qwenRe.MatchString(rc.Miss) {
				fmt.Printf("    ⚠️  Qwen: matches 'miss' string (should not)\n")
				qwenMissMismatchCount++
			}
		}

		validCases = append(validCases, rc)
	}

	fmt.Printf("\n=== Benchmarking %d valid patterns (%d skipped) ===\n", len(validCases), skippedCount)
	fmt.Printf("GPT optimized mismatches: %d match failures, %d miss failures\n", gptMatchMismatchCount, gptMissMismatchCount)
	fmt.Printf("Mistral optimized mismatches: %d match failures, %d miss failures\n", mistralMatchMismatchCount, mistralMissMismatchCount)
	fmt.Printf("Qwen optimized mismatches: %d match failures, %d miss failures\n\n", qwenMatchMismatchCount, qwenMissMismatchCount)

	for _, rc := range validCases {
		// Benchmark original pattern
		source := newSource(rc.Type, rc.ReplacePlaceholder, rc.Pattern)
		processor := &Processor{}

		// Benchmark with Match string
		if rc.Match != "" {
			b.Run(rc.Name+"/match/original", func(b *testing.B) {
				content := []byte(rc.Match)
				msg := newMessage(content, &source, "")

				b.ReportAllocs()
				b.ResetTimer()

				for b.Loop() {
					msg.SetContent(content)
					processor.applyRedactingRules(msg)
				}
			})
		}

		// Benchmark with Miss string
		if rc.Miss != "" {
			b.Run(rc.Name+"/miss/original", func(b *testing.B) {
				content := []byte(rc.Miss)
				msg := newMessage(content, &source, "")

				b.ReportAllocs()
				b.ResetTimer()

				for b.Loop() {
					msg.SetContent(content)
					processor.applyRedactingRules(msg)
				}
			})
		}

		// Benchmark GPT optimized pattern if present
		if rc.GptOptimized != "" {
			gptSource := newSource(rc.Type, rc.ReplacePlaceholder, rc.GptOptimized)
			gptProcessor := &Processor{}

			if rc.Match != "" {
				b.Run(rc.Name+"/match/gpt", func(b *testing.B) {
					content := []byte(rc.Match)
					msg := newMessage(content, &gptSource, "")
					b.ReportAllocs()
					b.ResetTimer()
					for b.Loop() {
						msg.SetContent(content)
						gptProcessor.applyRedactingRules(msg)
					}
				})
			}

			if rc.Miss != "" {
				b.Run(rc.Name+"/miss/gpt", func(b *testing.B) {
					content := []byte(rc.Miss)
					msg := newMessage(content, &gptSource, "")
					b.ReportAllocs()
					b.ResetTimer()
					for b.Loop() {
						msg.SetContent(content)
						gptProcessor.applyRedactingRules(msg)
					}
				})
			}
		}

		// Benchmark Mistral optimized pattern if present
		if rc.MistralOptimized != "" {
			mistralSource := newSource(rc.Type, rc.ReplacePlaceholder, rc.MistralOptimized)
			mistralProcessor := &Processor{}

			if rc.Match != "" {
				b.Run(rc.Name+"/match/mistral", func(b *testing.B) {
					content := []byte(rc.Match)
					msg := newMessage(content, &mistralSource, "")
					b.ReportAllocs()
					b.ResetTimer()
					for b.Loop() {
						msg.SetContent(content)
						mistralProcessor.applyRedactingRules(msg)
					}
				})
			}

			if rc.Miss != "" {
				b.Run(rc.Name+"/miss/mistral", func(b *testing.B) {
					content := []byte(rc.Miss)
					msg := newMessage(content, &mistralSource, "")
					b.ReportAllocs()
					b.ResetTimer()
					for b.Loop() {
						msg.SetContent(content)
						mistralProcessor.applyRedactingRules(msg)
					}
				})
			}
		}

		// Benchmark Qwen optimized pattern if present
		if rc.QwenOptimized != "" {
			qwenSource := newSource(rc.Type, rc.ReplacePlaceholder, rc.QwenOptimized)
			qwenProcessor := &Processor{}

			if rc.Match != "" {
				b.Run(rc.Name+"/match/qwen", func(b *testing.B) {
					content := []byte(rc.Match)
					msg := newMessage(content, &qwenSource, "")
					b.ReportAllocs()
					b.ResetTimer()
					for b.Loop() {
						msg.SetContent(content)
						qwenProcessor.applyRedactingRules(msg)
					}
				})
			}

			if rc.Miss != "" {
				b.Run(rc.Name+"/miss/qwen", func(b *testing.B) {
					content := []byte(rc.Miss)
					msg := newMessage(content, &qwenSource, "")
					b.ReportAllocs()
					b.ResetTimer()
					for b.Loop() {
						msg.SetContent(content)
						qwenProcessor.applyRedactingRules(msg)
					}
				})
			}
		}
	}
}
