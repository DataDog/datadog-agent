package profile

func mergeProfiles(profilesA ProfileConfigMap, profilesB ProfileConfigMap) ProfileConfigMap {
	profiles := make(ProfileConfigMap)
	for k, v := range profilesA {
		profiles[k] = v
	}
	for k, v := range profilesB {
		profiles[k] = v
	}
	return profiles
}
