package windowsutils

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

func WindowsWMIMultilineCommand(option string, names ...string) (result []map[string]string, err error) {
	/*
		Execute the WMI command `option`, and gathers the `names` fields for each
		non empty output line.
	*/
	out, err := exec.Command("wmic.exe", option).Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) < 2 {
		return nil, nil
	}

	// remove empty lines
	nonEmptyLines := 0
	lenLines := 0
	var line string
	for {
		lenLines = len(lines)
		if nonEmptyLines >= lenLines {
			break
		}
		line = lines[nonEmptyLines]
		if len(line) > 2 { // non empty and non "\n"
			nonEmptyLines++
		} else {
			lines[nonEmptyLines] = lines[lenLines-1]
			lines = lines[:lenLines-1]
		}
	}

	// build result
	result = make([]map[string]string, len(lines)-1)
	for i, line := range lines[1:] {
		result[i] = WindowsWMIFields(lines[0], line, names)
	}

	return
}

func WindowsWMICommand(option string, names ...string) (map[string]string, error) {
	/*
	   Execute the WMI command `option`, and extracts the `names` fields for the
	   first output.
	*/
	out, err := WindowsWMIMultilineCommand(option, names...)
	return out[0], err
}

func WindowsWMIFields(headers string, values string, names []string) (results map[string]string) {
	/*
		Parses a wmi result.
		`headers` is the header line of the wmi result
		`values` is the considered line to parse
		`names` are the values extracted
		eg.
		Access Automount Availability BlockSize
		       TRUE                   2048
	*/
	results = make(map[string]string)
	namesLen := len(names)
	if namesLen == 0 {
		return
	}
	var expectedColumnIndexes = make([]int, namesLen)

	sort.Strings(names) // sort names to extract since headers are in alphabetical order
	headerColumns := strings.Fields(headers)

	nameIndex := 0
	name := names[nameIndex]
	done := false
	for i, col := range headerColumns {
		if col == name {
			expectedColumnIndexes[nameIndex] = i
			nameIndex++
			if nameIndex >= len(names) {
				done = true
				break
			}
			name = names[nameIndex]
		}
	}
	if done == false || len(values) < expectedColumnIndexes[namesLen-1] {
		return
	}

	var letterIndex int
	var headerName string
	for i, name := range names {
		// look for the index of " NAME "
		headerName = fmt.Sprintf(" %s ", headerColumns[expectedColumnIndexes[i]])
		letterIndex = strings.Index(headers, headerName) + 1
		columnWidth := strings.Index(headers[letterIndex:], headerColumns[expectedColumnIndexes[i]+1])
		results[name] = strings.Trim(values[letterIndex:letterIndex+columnWidth], " ")
		// results[name] = nextWord(values, letterIndex)
	}

	return
}

func nextWord(line string, index int) string {
	/*
		Retrieve the current word given an index and a string
		"abc 2048   def ", 4 -> "2048"
	*/
	lastLetter := strings.Index(line[index:], " ")
	if lastLetter == -1 {
		return line[index:]
	}
	return line[index : index+lastLetter]
}
