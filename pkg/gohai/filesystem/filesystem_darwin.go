package filesystem

var dfOptions = []string{"-l", "-k"}
var expectedLength = 9

func updatefileSystemInfo(values []string) map[string]string {
	return map[string]string{
		"name":       values[0],
		"kb_size":    values[1],
		"mounted_on": values[8],
	}
}
