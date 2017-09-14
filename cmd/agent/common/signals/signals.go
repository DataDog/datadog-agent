package signals

// This is a new package in order to avoid cyclical imports

// Stopper is the channel used by other packages to ask for stopping the agent
var Stopper = make(chan bool)
var ErrorStopper = make(chan bool)
