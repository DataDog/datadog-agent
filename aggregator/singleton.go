package aggregator

var _aggregator Aggregator

func Get() Aggregator {
	return _aggregator
}

func Set(aggregatorInstance Aggregator) {
	_aggregator = aggregatorInstance
}
