package filesystem

var dfOptions = []string{"-l"}
var expectedLength = 6

func updatefileSystemInfo(values []string) (map[string]string, error) {
	return map[string]string{
		"name":       values[0],
		"kb_size":    values[1],
		"mounted_on": values[5],
	}, nil
}
