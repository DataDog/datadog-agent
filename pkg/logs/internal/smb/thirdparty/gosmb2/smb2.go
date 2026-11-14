// Package smb2 implements the SMB2/3 client in [MS-SMB2].
//
// https://msdn.microsoft.com/en-us/library/cc246482.aspx
//
// This package doesn't support CAP_UNIX extension.
// Symlink is supported by FSCTL_SET_REPARSE_POINT and FSCTL_GET_REPARSE_POINT.
// The symlink-following algorithm is explained in 2.2.2.2.1 and 2.2.2.2.1.1.
//
// https://msdn.microsoft.com/en-us/library/cc246542.aspx
//
// Supported features and protocol versions are declared in feature.go.
package smb2
