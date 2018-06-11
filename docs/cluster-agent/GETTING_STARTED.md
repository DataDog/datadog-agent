# Datadog Cluster Agent - DCA | Getting Started

## Introduction

For more context on the Datadog Cluster agent, please refer to the [User Documentation](README.md).
For more technical background, refer to [this documentation](../../Dockerfiles/cluster-agent/README.md).

## Step by step

The DCA will need a proper RBAC to be up and running, so you will first need to create the:
- Service Account
- Cluster Role
- Cluster Role Binding

These can be found [here](../../Dockerfiles/manifests/cluster-agent/rbac/rbac-cluster-agent.yaml).

Simply run: `kubectl apply -f Dockerfiles/manifests/cluster-agent/rbac-cluster-agent.yaml` from the datadog-agent directory.

Then, depending on whether you are relying on a secret to secure the communication between the Node Agent and the Cluster Agent, you will either:
- Create a secret
- Set an environment variable

To create a secret, you can create a 32 characters long base64 encoded string:
`echo -n <Thirty 2 characters long token> | base64`
and use this string in the `dca-secret.yaml` file located in the [manifest/cluster-agent/](../../Dockerfiles/manifests/cluster-agent/dca-secret.yaml) directory.
Or you can run a one line command: `kubectl create secret generic datadog-auth-token --from-literal=token=12345678901234567890123456789012`.

Upon creation you need to refer to this secret in the manifest of the cluster agent as well as the agent!
The environment variable `DD_CLUSTER_AGENT_AUTH_TOKEN` shall be used for this purpose.
If you are using the secret:
```yaml
          - name: DD_CLUSTER_AGENT_AUTH_TOKEN
            valueFrom:
              secretKeyRef:
                name: datadog-auth-token
                key: token

```
 Or otherwise:
 ```yaml
          - name: DD_CLUSTER_AGENT_AUTH_TOKEN
            value: "12345678901234567890123456789012"
```

Again, this needs to be set in the manifest of the cluster agent *and* the node agent.

Once the secret is created, create the DCA along with its service.
Don't forget to add your API_KEY in the manifest of the DCA. 
Both manifests can be found in the manifest/cluster-agent directory.
`kubectl apply -f Dockerfiles/manifests/cluster-agent/datadog-cluster-agent_service.yaml`
and
`kubectl apply -f Dockerfiles/manifests/cluster-agent/cluster-agent.yaml`

At this point, you should be seeing:
```yaml
-> kubectl get deploy
NAME                    DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
datadog-cluster-agent   1         1         1            1           1d

-> kubectl get secret
NAME                   TYPE                                  DATA      AGE
datadog-auth-token     Opaque                                1         1d

-> kubectl get pods | grep cluster
datadog-cluster-agent-8568545574-x9tc9   1/1       Running   0          2h

-> kubectl get service
NAME                  TYPE           CLUSTER-IP       EXTERNAL-IP        PORT(S)          AGE
dca                   ClusterIP      10.100.202.234   <none>             5005/TCP         1d
```

The last step is to configure your agent to communicate with the DCA.
First, you will need to create the RBAC for your agents. They limit the agents' access  to the kubelet API.
You will need to create a dedicated:
- Service Account
- Cluster Role
- Cluster Role Binding
These can be found [here](../../Dockerfiles/manifests/cluster-agent/rbac/rbac-agent.yaml).

Simply run: `kubectl apply -f Dockerfiles/manifests/cluster-agent/rbac-agent.yaml` from the datadog-agent directory.

To do so, add the following environment variables to the agent's manifest:
```yaml
          - name: DD_CLUSTER_AGENT
            value: 'true'
          - name: DD_CLUSTER_AGENT_AUTH_TOKEN
            valueFrom:
              secretKeyRef:
                name: datadog-auth-token
                key: token 
#            value: "12345678901234567890123456789012" # If you are not using the secret, just set the string.                
          - name: DD_KUBERNETES_METADATA_TAG_UPDATE_FREQ # Optional
            value: '15'
```

And finally, create the Daemonsets for your agents:

`kubectl apply -f Dockerfiles/manifests/agent.yaml` 

You should be seeing:
```
-> kubectl get pods | grep agent
datadog-agent-4k9cd                      1/1       Running   0          2h
datadog-agent-4v884                      1/1       Running   0          2h
datadog-agent-9d5bl                      1/1       Running   0          2h
datadog-agent-dtlkg                      1/1       Running   0          2h
datadog-agent-jllww                      1/1       Running   0          2h
datadog-agent-rdgwz                      1/1       Running   0          2h
datadog-agent-x5wk5                      1/1       Running   0          2h
[...]
datadog-cluster-agent-8568545574-x9tc9   1/1       Running   0          2h
```

Then, Kubernetes events should start to flow in your accounts, and relevant metrics collected by your agents should be tagged with their corresponding cluster level metadata.


## Troubleshooting

#### On the DCA side
If you want to see what cluster level metadata is served by the DCA you can exec in the pod and run:
```
root@datadog-cluster-agent-8568545574-x9tc9:/# datadog-cluster-agent metamap

==============
Service Mapper
==============

Node detected: ip-192-168-114-181.ec2.internal

 -  Pod name: kube-state-metrics-5ffc474d8-225lc
    Services list: [kube-state-metrics]
 -  Pod name: redis-slave-8wdc5
    Services list: [redis-slave]
Node detected: ip-192-168-118-166.ec2.internal

 -  Pod name: guestbook-6gk2f
    Services list: [guestbook]
 -  Pod name: nginx-6757dd8769-kmg4l
    Services list: [nginx]
 -  Pod name: nginx-deployment-5bd546754d-5bg7z
    Services list: [nginx]
 -  Pod name: nginx-deployment-5bd546754d-brcwr
 
 [...]
```

If you want to verify that the DCA is being queried, you can look for:
```
root@datadog-cluster-agent-8568545574-x9tc9:/# tail -f /var/log/datadog/cluster-agent.log
2018-06-11 09:37:20 UTC | DEBUG | (metadata.go:40 in GetPodMetadataNames) | CacheKey: agent/KubernetesMetadataMapping/ip-192-168-226-77.ec2.internal, with 1 services
2018-06-11 09:37:20 UTC | DEBUG | (metadata.go:40 in GetPodMetadataNames) | CacheKey: agent/KubernetesMetadataMapping/ip-192-168-226-77.ec2.internal, with 1 services
```

If you are not collecting events properly, make sure you have enabled: 
- The leader election `DD_LEADER_ELECTION`
- The event collection `DD_COLLECT_KUBERNETES_EVENTS`

As well as the proper verbs listed in the RBAC (notably, watch events).

If you have enabled those, you can check the Leader Election status and the kube_apiserver check.

```yaml

root@datadog-cluster-agent-8568545574-x9tc9:/# datadog-cluster-agent status
[...]
  Leader Election
  ===============
    Leader Election Status:  Running
    Leader Name is: datadog-cluster-agent-8568545574-x9tc9
    Last Acquisition of the lease: Mon, 11 Jun 2018 06:38:53 UTC
    Renewed leadership: Mon, 11 Jun 2018 09:41:34 UTC
    Number of leader transitions: 2 transitions
[...]    
  Running Checks
  ==============
    kubernetes_apiserver
    --------------------
      Total Runs: 736
      Metrics: 0, Total Metrics: 0
      Events: 0, Total Events: 100
      Service Checks: 3, Total Service Checks: 2193    
[...]
```



#### On the Node Agent side:

Make sure the Cluster Agent service was created before the agents' pods, so that the DNS is available in the env vars:

```
root@datadog-agent-9d5bl:/# env | grep -i dca
DCA_SERVICE_PORT=5005
DCA_SERVICE_HOST=10.100.202.234
DCA_PORT_5005_TCP_PORT=5005
DCA_PORT=tcp://10.100.202.234:5005
DCA_PORT_5005_TCP=tcp://10.100.202.234:5005
DCA_PORT_5005_TCP_PROTO=tcp
DCA_PORT_5005_TCP_ADDR=10.100.202.234

root@datadog-agent-9d5bl:/# env | grep -i auth_token
DD_CLUSTER_AGENT_AUTH_TOKEN=1234****9
```

You can verify that the Node Agent is using the DCA as a tag provider:
```
root@datadog-agent-9d5bl:/# cat /var/log/datadog/agent.log | grep "metadata-collector"
2018-06-11 06:59:02 UTC | INFO | (tagger.go:151 in tryCollectors) | kube-metadata-collector tag collector successfully started
```

Or look for error logs, such as:
```
2018-06-10 08:03:02 UTC | ERROR | Could not initialise the communication with the DCA, falling back to local service mapping: [...]
```
