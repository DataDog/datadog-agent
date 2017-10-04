## DataDog Agent 6 GUI
A cross-platform GUI served on localhost:8080 (by default) for interacting with DataDog Agent 6.

#### Requirements
1. Cookies must be enabled in your browser. Upon initially accessing the GUI, you will be asked once to enter your API key; after that, the key is saved as a cookie and is used for authenticating all communications with the GUI server.  

2. The GUI allows you to edit your configuration (yaml) files. Therefore it is necessary for you to ensure that the user running the DataDog Agent has the correct permissions for writing to these files - generally this means that the user must be the owner.

#### Accessing the GUI from a VM
If you're running the agent on a VM (or some other kind of container), you need to have port forwarding set up in order to access the GUI from a browser on your host.  
If you're using vagrant, this is as easy as adding  
```config.vm.network :forwarded_port, guest: 8080, host: 8080```  
to the Vagrantfile. (If you change the default port for the GUI in datadog.yaml, replace 8080 by your chosen port.)

#### Disabling the GUI
The GUI can be disabled altogether by setting its port to -1 in your datadog.yaml file.

#### Known Bugs
1. When you reset a configuration file and remove an instance, the instance will indeed stop running - however, it still shows up in the Collector status (and in the list of running checks)
2. Reloading the default checks shouldn't work, it's not implemented yet
