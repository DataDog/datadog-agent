// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

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
	for i, product := range products {
		stringProducts[i] = string(product)
	}
	return stringProducts
}

// StringListToProduct converts a string list to product list
func StringListToProduct(stringProducts []string) []Product {
	products := make([]Product, len(stringProducts))
	for i, product := range stringProducts {
		products[i] = Product(product)
	}
	return products
}
