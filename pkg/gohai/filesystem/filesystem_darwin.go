package filesystem

import "fmt"

var dfOptions = []string{"-l", "-k"}
var expectedLength = 6

func updatefileSystemInfo(values []string) (map[string]string, error) {
	// On some MacOS systems df outputs 9 columns, on others 6
	if len(values) == 9 {
		return map[string]string{
			"name":       values[0],
			"kb_size":    values[1],
			"mounted_on": values[8],
		}, nil
	} else if len(values) == 6 {
		return map[string]string{
			"name":       values[0],
			"kb_size":    values[1],
			"mounted_on": values[5],
		}, nil
	}
	return nil, fmt.Errorf("unexpected df output with %d columns", len(values))
}
