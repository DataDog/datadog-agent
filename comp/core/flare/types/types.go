// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types contains all the types needed by FlareProviders without the underlying implementation and dependencies.
// This allows components to offer flare capabilities without linking to the flare dependencies when the flare feature
// is not built in the binary.
package types

import (
	"go.uber.org/fx"
)

// FlareBuilder contains all the helpers to add files to a flare archive.
//
// When adding data to a flare the builder will do multiple things internally.
//
// First a log of the archive creation will be kept and shipped with the flare. When using the the FlareBuilder you
// should not stop at the first error. We want to collect as much as possible. Any error returned by the a FlareBuilder
// method is added to the flare log. In general, you can safely ignore those errors unless sending a flare without some
// file/information would not make sense.
//
// The builder will automatically scrub any sensitive data from content and copied files. Still carefully analyze what
// is being added to ensure that it contains no credentials or unnecessary user-specific data. The FlareBuilder scrubs
// secrets that match pre-programmed patterns, but it is always better to not capture data containing secrets, than to
// scrub that data.
//
// Everytime a file is copied to the flare the original permissions and ownership of the file is recorded (Unix only).
//
// There are reserved path in the flare: "permissions.log" and "flare-creationg.log" (both at the root of the flare).
// Note as well that the flare does nothing to prevent files to be overwritten by different calls. It's up to the caller
// to make sure the path used in the flare doesn't clash with other modules.
type FlareBuilder interface {
	// IsLocal returns true when the flare is created by the CLI instead of the running Agent process. This happens
	// when the CLI could not reach the Agent process to request a new flare. In that case a flare is still created
	// directly from the CLI process and will not contains any runtime informations.
	IsLocal() bool

	// AddFile creates a new file in the flare with the content.
	//
	// 'destFile' is a path relative to the flare root (ex: "some/path/to/a/file"). Any necessary directory will
	// automatically be created.
	//
	// 'content' is automatically scrubbed of any sensitive informations before being added to the flare.
	AddFile(destFile string, content []byte) error

	// AddFileWithoutScrubbing creates a new file in the flare with the content.
	//
	// 'destFile' is a path relative to the flare root (ex: "some/path/to/a/file"). Any necessary directory will
	// automatically be created.
	//
	// 'content' is NOT scrubbed of any sensitive informations before being added to the flare.
	// Can be used for binary files that mustn’t be corrupted, like pprof profiles for ex.
	AddFileWithoutScrubbing(destFile string, content []byte) error

	// AddFileFromFunc creates a new file in the flare with the content returned by the callback.
	//
	// 'destFile' is a path relative to the flare root (ex: "some/path/to/a/file"). Any necessary directory will
	// automatically be created.
	//
	// If the 'cb' returns an error, the file will not be created, the error is added to the flare's logs and returned to
	// the caller.
	//
	// The data returned by 'cb' is automatically scrubbed of any sensitive informations before being added to the flare.
	AddFileFromFunc(destFile string, cb func() ([]byte, error)) error

	// CopyFile copies the content of 'srcFile' to the root of the flare.
	//
	// The data is automatically scrubbed of any sensitive informations before being copied.
	//
	// Example: CopyFile("/etc/datadog/datadog.yaml") will create a copy of "/etc/datadog/datadog.yaml", named
	// "datadog.yaml", at the root of the flare.
	CopyFile(srcFile string) error

	// CopyFileTo copies the content of 'srcFile' to 'destFile' in the flare.
	//
	// The data is automatically scrubbed of any sensitive informations before being copied.
	//
	// 'destFile' is a path relative to the flare root (ex: "path/to/a/file"). Any necessary directory will
	// automatically be created.
	//
	// Example: CopyFile("/etc/datadog/datadog.yaml", "etc/datadog.yaml") will create a copy of "/etc/datadog/datadog.yaml"
	// at "etc/datadog.yaml" at the root of the flare.
	CopyFileTo(srcFile string, destFile string) error

	// CopyDirTo copies files from the 'srcDir' to a specific directory in the flare.
	//
	// The path for each file in 'srcDir' is passed to the 'shouldInclude' callback. If 'shouldInclude' returns true, the
	// file is copies to the flare. If not, the file is ignored.
	//
	// 'destDir' is a path relative to the flare root (ex: "some/path/to/a/dir").
	//
	// The data of each copied file is automatically scrubbed of any sensitive informations before being copied.
	//
	// Example: CopyDir("/var/log/datadog/agent", "logs", <callback>) will copy files from "/var/log/datadog/agent/" to
	// "logs/agent/" in the flare.
	CopyDirTo(srcDir string, destDir string, shouldInclude func(string) bool) error

	// CopyDirTo copies files from the 'srcDir' to a specific directory in the flare.
	//
	// The path for each file in 'srcDir' is passed to the 'shouldInclude' callback. If 'shouldInclude' returns true, the
	// file is copies to the flare. If not, the file is ignored.
	//
	// 'destDir' is a path relative to the flare root (ex: "some/path/to/a/dir").
	//
	// The data of each copied file is NOT scrubbed of any sensitive informations before being copied. Only files
	// already scrubbed should be added in this way (ex: agent logs that are scrubbed at creation).
	//
	// Example: CopyDir("/var/log/datadog/agent", "logs", <callback>) will copy files from "/var/log/datadog/agent/" to
	// "logs/agent/" in the flare.
	CopyDirToWithoutScrubbing(srcDir string, destDir string, shouldInclude func(string) bool) error

	// PrepareFilePath returns the full path of a file in the flare.
	//
	// PrepareFilePath will create the necessary directories in the flare temporary dir so that file can be create, but will
	// not create the file. This method should only be used when the data is generated by another program/library.
	//
	// Example: PrepareFilePath("db-monitoring/db-dump.log") will create the 'db-monitoring' directory at the root of the
	// flare and return the full path to db-dump.log: "/path/to/the/flare/db-monitoring/db-dump.log".
	PrepareFilePath(path string) (string, error)

	// RegisterFilePerm add the current permissions for a file to the flare's permissions.log.
	RegisterFilePerm(path string)

	// RegisterDirPerm add the current permissions for all the files in a directory to the flare's permissions.log.
	RegisterDirPerm(path string)

	// Save archives all the data added to the flare, cleanup all the temporary directories and return the path to
	// the archive file. Upon error the cleanup is still done.
	// Error or not, once Save as been called the FlareBuilder is no longer capable of receiving new data. It is the caller
	// responsibility to make sure of this.
	//
	// This method must not be used by flare callbacks and will be removed once all flare code has been migrated to
	// components.
	Save() (string, error)
}

// FlareCallback is a function that can be registered as a data provider for flares. This function, if registered, will
// be called everytime a flare is created.
type FlareCallback func(fb FlareBuilder) error

// FlareProvider represents a callback to be used when creating a flare
type FlareProvider struct {
	Callback FlareCallback
}

// Provider is provided by other components to register themselves to provide flare data.
type Provider struct {
	fx.Out

	Provider FlareProvider `group:"flare"`
}

// NewProvider returns a new Provider to be called when a flare is created
func NewProvider(callback FlareCallback) Provider {
	return Provider{
		Provider: FlareProvider{
			Callback: callback,
		},
	}
}
