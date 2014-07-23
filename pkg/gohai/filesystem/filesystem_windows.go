package filesystem

import (
	utils "github.com/DataDog/gohai/windowsutils"
	"strconv"
)

func getFileSystemInfo() (interface{}, error) {

	volumes, err := utils.WindowsWMIMultilineCommand("VOLUME", "Name", "Capacity", "DriveLetter")
	if err != nil {
		return nil, err
	}
	var fileSystemInfo = make([]interface{}, len(volumes))
	for i, volume := range volumes {
		var capacity string
		intCapacity, err := strconv.ParseInt(volume["Capacity"], 10, 64)
		if err != nil {
			capacity = "Unknown"
		} else {
			capacity = strconv.FormatInt(intCapacity/1024.0, 10)
		}
		fileSystemInfo[i] = map[string]interface{}{
			"name":       volume["Name"],
			"kb_size":    capacity,
			"mounted_on": volume["DriveLetter"],
		}
	}

	return fileSystemInfo, nil
}
