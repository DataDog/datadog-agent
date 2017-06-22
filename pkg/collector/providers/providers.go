package providers

// ProviderCatalog keeps track of Go loaders by name
var ProviderCatalog = make(map[string]ConfigProvider)

// RegisterProvider adds a loader to the loaderCatalog
func RegisterProvider(name string, p ConfigProvider) {
	ProviderCatalog[name] = p
}
