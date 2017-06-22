package providers

// LoaderCatalog keeps track of Go loaders by name
var ProviderCatalog = make(map[string]ConfigProvider)

// RegisterLoader adds a loader to the loaderCatalog
func RegisterProvider(name string, p ConfigProvider) {
	ProviderCatalog[name] = p
}
