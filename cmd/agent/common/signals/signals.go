package signals

// This is a new package in order to avoid cyclical imports

var (
	// Stopper is the channel used by other packages to ask for stopping the agent
	Stopper = make(chan bool)

	// ErrorStopper is the channel used by other packages to ask for stopping the agent because of an error
	ErrorStopper = make(chan bool)
)
