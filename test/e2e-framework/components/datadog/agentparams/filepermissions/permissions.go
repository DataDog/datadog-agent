package filepermissions

// FilePermissions interface defines all commands that can be used to set and reset file permissions.
// These functions are used to create args.Command that will be run by pulumi.
type FilePermissions interface {
	SetupPermissionsCommand(path string) string
	ResetPermissionsCommand(path string) string
}
