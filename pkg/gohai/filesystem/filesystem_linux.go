package filesystem

var dfOptions = []string{"-l"}
var expectedLength = 6

func updatefileSystemInfo(fileSystemInfo map[string]interface{}, values []string) {
	name := values[0]
	fileSystemInfo[name] = map[string]string{
		"kb_size":    values[1],
		"mounted_on": values[5],
	}
}
