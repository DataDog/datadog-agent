package filesystem

import (
	utils "github.com/DataDog/gohai/windowsutils"
)

func getFileSystemInfo() (interface{}, error) {

	volumes, err := utils.WindowsWMIMultilineCommand("VOLUME", "Name", "Capacity", "DriveLetter")
	if err != nil {
		return nil, err
	}
	var fileSystemInfo = make([]interface{}, len(volumes))
	for i, volume := range volumes {
		fileSystemInfo[i] = map[string]interface{}{
			"name":       volume["Name"],
			"kb_size":    volume["Capacity"],
			"mounted_on": volume["DriveLetter"],
		}
	}

	return fileSystemInfo, nil
}
