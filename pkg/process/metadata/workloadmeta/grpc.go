package workloadmeta

type mockableGrpcListener interface {
	writeEvents(procsToDelete, procsToAdd []*ProcessEntity)
}

var _ mockableGrpcListener = (*grpcListener)(nil)

type grpcListener struct {
	evts chan *ProcessEntity
}

func newGrpcListener() *grpcListener {
	return &grpcListener{
		evts: make(chan *ProcessEntity, 0),
	}
}

func (l *grpcListener) writeEvents(procsToDelete, procsToAdd []*ProcessEntity) {
	// TODO
}
