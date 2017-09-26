## DataDog Agent 6 GUI

A cross-platform GUI served on localhost:8080 (by default) for interacting with DataDog Agent 6.

#### Requirements

The GUI allows the user to edit their configuration (yaml) files. Therefore it is necessary for the user to ensure that the user running the DataDog Agent has the correct permissions for writing to these files - generally this means that the user must be the owner.

#### Accessing the GUI from a VM

If you're running the agent on a VM (or some other kind of container), you need to have port forwarding set up in order to access the GUI from a browser on your host.  
If you're using vagrant, this is as easy as adding  
```config.vm.network :forwarded_port, guest: 8080, host: 8080```  
to the Vagrantfile. (If you change the default port for the GUI in datadog.yaml, replace 8080 by your chosen port.)
