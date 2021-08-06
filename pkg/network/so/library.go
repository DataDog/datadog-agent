package so

type libraryKey struct {
	Pathname       string
	MountNameSpace ns
}

type Library struct {
	libraryKey
	PidsPath  []string
	MountInfo *mountInfo
	HostPath  string
}
