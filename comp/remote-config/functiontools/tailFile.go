package functiontools

import (
	"bufio"
	"os"
)

func tailFile(path string, n_lines float64) ([]string, error) {
	n := int(n_lines)

	// Open the file
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Read the file line by line
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// If the number of lines to read is greater than the number of lines in the file, return all lines
	if n >= len(lines) {
		return lines, nil
	}

	// Return the last N lines of the file
	return lines[len(lines)-n:], nil
}
