package status

import (
	"encoding/json"
	"html/template"
	"strings"
)

func doNotEscape(value string) template.HTML {
	return template.HTML(value)
}

func pythonLoaderError(value string) template.HTML {
	value = strings.Replace(value, "', '", "", -1)
	value = strings.Replace(value, "['", "", -1)
	value = strings.Replace(value, "\\n']", "", -1)
	value = strings.Replace(value, "']", "", -1)
	value = strings.Replace(value, "\\n", "\n      ", -1)
	var loaderErrorArray []string
	json.Unmarshal([]byte(value), &loaderErrorArray)
	return template.HTML(value)
}

func lastError(value string) template.HTML {
	return template.HTML(value)
}

func lastErrorTraceback(value string) template.HTML {
	var lastErrorArray []map[string]string

	json.Unmarshal([]byte(value), &lastErrorArray)
	lastErrorArray[0]["traceback"] = strings.Replace(lastErrorArray[0]["traceback"], "\n", "\n      ", -1)
	return template.HTML(lastErrorArray[0]["traceback"])
}

func lastErrorMessage(value string) template.HTML {
	var lastErrorArray []map[string]string

	json.Unmarshal([]byte(value), &lastErrorArray)
	return template.HTML(lastErrorArray[0]["message"])
}
