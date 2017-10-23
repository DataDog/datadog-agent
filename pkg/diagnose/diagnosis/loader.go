package diagnosis

// Catalog holds available diagnosis for detection and usage
type Catalog map[string]Diagnosis

// DefaultCatalog holds every compiled-in diagnosis
var DefaultCatalog = make(Catalog)

// Register a diagnosis that will be called on diagnose
func Register(name string, d Diagnosis) {
	DefaultCatalog[name] = d
}

// Diagnosis should implement Diagnose to report status
type Diagnosis interface {
	Diagnose() (err error)
}
