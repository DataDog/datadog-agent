package os

// Implements commonly used descriptors for easier usage
// See platforms.go for the AMIs used for each OS
var (
	MacOSDefault = MacOSSonoma
	MacOSSonoma  = NewDescriptor(MacosOS, "sonoma")
)

var MacOSDescriptorsDefault = map[Flavor]Descriptor{
	MacosOS: MacOSDefault,
}
