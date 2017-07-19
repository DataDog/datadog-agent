package providers

// ProviderCatalog keeps track of config providers by name
var ProviderCatalog = make(map[string]ConfigProvider)

// RegisterProvider adds a loader to the providers catalog
func RegisterProvider(name string, p ConfigProvider) {
	ProviderCatalog[name] = p
}
