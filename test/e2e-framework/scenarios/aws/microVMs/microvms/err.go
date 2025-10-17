package microvms

import "errors"

var (
	ErrVMSetsNotMapped = errors.New("vmsets must be mapped to collection before building pools")
	ErrInvalidPoolSize = errors.New("ram backed pool must have size specified in megabytes or gigabytes with the appropriate suffix 'M' or 'G'")
	ErrZeroRAMDiskSize = errors.New("ram disk size not provided")
)
