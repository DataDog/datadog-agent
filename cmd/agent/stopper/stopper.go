package stopper

// Stopper is the channel used by other packages to ask for stopping the agent
var Stopper = make(chan bool)
