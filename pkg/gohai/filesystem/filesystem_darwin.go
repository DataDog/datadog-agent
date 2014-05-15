package filesystem

var dfOptions = []string{"-l", "-k"}
var expectedLength = 9

func updatefileSystemInfo(fileSystemInfo map[string]interface{}, values []string) {
	name := values[0]
	fileSystemInfo[name] = map[string]string{
		"kb_size":    values[1],
		"mounted_on": values[8],
	}
}
