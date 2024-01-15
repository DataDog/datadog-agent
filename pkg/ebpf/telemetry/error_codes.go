package telemetry

// BTFResult enumerates BTF loading success & failure modes
type BTFResult int

const (
	SuccessCustomBTF   BTFResult = 0
	SuccessEmbeddedBTF BTFResult = 1
	SuccessDefaultBTF  BTFResult = 2
	BtfNotFound        BTFResult = 3
)

// COREResult enumerates CO-RE success & failure modes
type COREResult int

const (
	// BTFResult comes beforehand

	AssetReadError COREResult = 4
	VerifierError  COREResult = 5
	LoaderError    COREResult = 6
)
