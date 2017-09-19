## DataDog Agent 6 GUI

A cross-platform GUI served on localhost:8080 (by default) for interacting with DataDog Agent 6.

#### Requirements
For the GUI to be able to run, you must have the DATADOG_ROOT environment variable set to whatever directory you have your datadog-agent folder in (ie ~/dev/go/src/github.com/DataDog).

#### Accessing the GUI from a VM

If you're running the agent on a VM (or some other kind of container), you need to have port forwarding set up in order to access the GUI from a browser on your host.  
If you're using vagrant, this is as easy as adding  
```config.vm.network :forwarded_port, guest: 8080, host: 8080```  
to the Vagrantfile. (If you change the default port for the GUI in datadog.yaml, replace 8080 by your chosen port.)

#### Sending requests to the go server (for devs)
In order for a request to be sent to the server and accepted, it must match the following specifications:
1. Must be a POST request
2. Must contain the correct API key (read from datadog.yaml) in the header. Format:  
```Authorization: 'Bearer ' + API_KEY```  
3. Must contain 2 fields:
    * req_type: the request type.
    * data: the data to be sent with the request (usually a keyword)

Available requests at this time:

| Req_type | Data                                               |
|----------|----------------------------------------------------|
| command  | check_running, start_agent, stop_agent, get_status |
|          |                                                    |
