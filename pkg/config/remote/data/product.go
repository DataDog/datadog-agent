package data

// Product is a remote configuration product
type Product string

const (
	// ProductAPMSampling is the apm sampling product
	ProductAPMSampling Product = "APM_SAMPLING"
	// ProductTesting1 is a testing product
	ProductTesting1 Product = "TESTING1"
)

// ProductListToString converts a product list to string list
func ProductListToString(products []Product) []string {
	stringProducts := make([]string, len(products))
	for _, product := range products {
		stringProducts = append(stringProducts, string(product))
	}
	return stringProducts
}

// ProductListToString converts a product list to string list
func StringListToProduct(stringProducts []string) []Product {
	products := make([]Product, len(stringProducts))
	for _, product := range stringProducts {
		products = append(products, Product(product))
	}
	return products
}
