package profile

var globalProfileConfigMap ProfileConfigMap

func SetGlobalProfileConfigMap(configMap ProfileConfigMap) {
	globalProfileConfigMap = configMap
}

func GetGlobalProfileConfigMap() ProfileConfigMap {
	return globalProfileConfigMap
}
