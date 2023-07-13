package telemetry

// // MarshalJSON returns a json representation of the current `Metric`. We
// // implement our own method so we don't need to export the fields.
// // This is mostly inteded for serving a list of the existing
// // metrics under /network_tracer/debug/telemetry endpoint
// func (m *metricBase) MarshalJSON() ([]byte, error) {
// 	return json.Marshal(struct {
// 		Name string
// 		Tags []string `json:",omitempty"`
// 		Opts []string
// 	}{
// 		Name: m.name,
// 		Tags: m.tags.List(),
// 		Opts: m.opts.List(),
// 		Value: m
// 	})
// }
