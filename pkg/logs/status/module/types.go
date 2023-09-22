package module

// AddGlobalWarning keeps track of a warning message to display on the status.
type AddGlobalWarning func(key string, warning string)

// RemoveGlobalWarning loses track of a warning message
// that does not need to be displayed on the status anymore.
type RemoveGlobalWarning func(key string)
