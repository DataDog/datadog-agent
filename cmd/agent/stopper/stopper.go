// Package stopper provides a communication system to forward requests
// to stop the agent between related packages to avoid circular dependencies.
package stopper

// Stopper is the channel used by other packages to ask for stopping the agent
var Stopper = make(chan bool)
