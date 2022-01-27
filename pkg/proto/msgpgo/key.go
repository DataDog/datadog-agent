package msgpgo

// RemoteConfigKey is a RemoteConfigKey
type RemoteConfigKey struct {
	AppKey     string `msgpack:"key"`
	OrgID      int64  `msgpack:"org"`
	Datacenter string `msgpack:"dc"`
}
