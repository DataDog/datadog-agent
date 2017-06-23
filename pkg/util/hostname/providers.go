package hostname

// Provider is a generic function to grab the hostname and return it
type Provider func(string) (string, error)

// ProviderCatalog holds all the various kinds of hostname providers
var ProviderCatalog = make(map[string]Provider)

// RegisterHostnameProvider registers a hostname provider as part of the catalog
func RegisterHostnameProvider(name string, p Provider) {
	ProviderCatalog[name] = p
}
