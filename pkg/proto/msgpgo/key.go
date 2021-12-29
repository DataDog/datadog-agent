package msgpgo

type RemoteConfigKey struct {
	AppKey     string `msgpack:"key"`
	OrgId      int64  `msgpack:"org"`
	Datacenter string `msgpack:"dc"`
}
