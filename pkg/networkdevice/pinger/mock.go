package pinger

type mockPinger struct {
	res *Result
	err error
}

func NewMockPinger(res *Result, err error) *mockPinger {
	return &mockPinger{
		res: res,
		err: err,
	}
}

func (m *mockPinger) Ping(host string) (*Result, error) {
	return m.res, m.err
}
