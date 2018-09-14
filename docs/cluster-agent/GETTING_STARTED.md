# Datadog Cluster Agent | Getting Started

The Datadog Cluster Agent is a **beta** feature, if you are facing any issues please reach out to our [support team](http://docs.datadoghq.com/help)

## Introduction

This document aims at getting you started using the Datadog Cluster Agent. Refer to the [Datadog Cluster Agent | User Documentation](README.md) for more context about the Datadog Cluster agent. For more technical background, refer to the [Datadog Cluster Agent technical documentation](../../Dockerfiles/cluster-agent/README.md) documentation.

## Step by step

**Step 1** - The Datadog Cluster Agent needs a proper RBAC to be up and running:

* Service Account
* Cluster Role
* Cluster Role Binding

Review these manifests in the [Datadog Cluster Agent RBAC folder](../../Dockerfiles/manifests/cluster-agent/rbac).

**Step 2** - Run: `kubectl apply -f Dockerfiles/manifests/cluster-agent/rbac/rbac-cluster-agent.yaml` from the `datadog-agent` directory.

**Step 3** - Depending on whether you are relying on a secret to secure the communication between the Datadog Agent and the Datadog Cluster Agent, either:

* Create a secret and access it with an environment variable 
* Set a token in an environment variable

**Step 3.1** - To create a secret, create a 32 characters long base64 encoded string: `echo -n '<ThirtyX2XcharactersXlongXtoken>' | base64`
and use this string in the `dca-secret.yaml` file located in the [manifest/cluster-agent directory](../../Dockerfiles/manifests/cluster-agent/dca-secret.yaml). Alternately run this one line command: `kubectl create secret generic datadog-auth-token --from-literal=token=<ThirtyX2XcharactersXlongXtoken>`.

**Step 3.2** - Upon creation, refer to this secret with the environment variable `DD_CLUSTER_AGENT_AUTH_TOKEN`  in the manifest of the cluster agent as well as in the manifest of the Agent!

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
            value: "<ThirtyX2XcharactersXlongXtoken>"
```

Setting the value without a secret will result in the token being readable in the PodSpec.

**Note**: This needs to be set in the manifest of the cluster agent **AND** the node agent.

**Step 3 bis** - If you do not want to rely on environment variables, you can mount the datadog.yaml file. We recommend using a ConfigMap.
To do so, add the following in the manifest of the cluster agent:

```yaml
[...]
        volumeMounts:
        - name: "dca-yaml"
          mountPath: "/etc/datadog-agent/datadog.yaml"
          subPath: "datadog-cluster.yaml"
      volumes:
        - name: "dca-yaml"
          configMap:
            name: "dca-yaml"
[...]
```
Then create your datadog-cluster.yaml with the variables of your choice.
Create the ConfigMap accordingly:
`kubectl create configmap dca-yaml --from-file datadog-cluster.yaml`

**Step 4** - Once the secret is created, create the Datadog Cluster Agent along with its service.
Don't forget to add your `<DD_API_KEY>` in the manifest of the Datadog Cluster Agent. Both manifests can be found in the [manifest directory](https://github.com/DataDog/datadog-agent/tree/master/Dockerfiles/manifests)
Run: 

`kubectl apply -f Dockerfiles/manifests/cluster-agent/datadog-cluster-agent_service.yaml`

and

`kubectl apply -f Dockerfiles/manifests/cluster-agent/cluster-agent.yaml`

**Step 5** - At this point, you should be seeing:

```
-> kubectl get deploy
NAME                    DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
datadog-cluster-agent   1         1         1            1           1d

-> kubectl get secret
NAME                   TYPE                                  DATA      AGE
datadog-auth-token     Opaque                                1         1d

-> kubectl get pods -l app:datadog-cluster-agent
datadog-cluster-agent-8568545574-x9tc9   1/1       Running   0          2h

-> kubectl get service -l app:datadog-cluster-agent
NAME                    TYPE           CLUSTER-IP       EXTERNAL-IP        PORT(S)          AGE
datadog-cluster-agent   ClusterIP      10.100.202.234   none               5005/TCP         1d
```

**Step 6** - Configure your agent to communicate with the Datadog Cluster Agent.
First, create the RBAC for your agents. They limit the agents' access to the kubelet API.

- Service Account
- Cluster Role
- Cluster Role Binding

Review these manifests in these files [Datadog Agent RBAC](../../Dockerfiles/manifests/cluster-agent/rbac/rbac-agent.yaml).

**Step 7** - Run: `kubectl apply -f Dockerfiles/manifests/cluster-agent/rbac/rbac-agent.yaml` from the datadog-agent directory.

Then, enable the Datadog Cluster Agent. Add the following environment variables to the agent's manifest:

```yaml
          - name: DD_CLUSTER_AGENT_ENABLED
            value: 'true'
          - name: DD_CLUSTER_AGENT_AUTH_TOKEN
            valueFrom:
              secretKeyRef:
                name: datadog-auth-token
                key: token
#            value: "<ThirtyX2XcharactersXlongXtoken>" # If you are not using the secret, just set the string.
```

**Step 8** - Create the Daemonsets for your agents:

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

Kubernetes events should start to flow in your Datadog account, and relevant metrics collected by your agents should be tagged with their corresponding cluster level metadata.

## Troubleshooting
<a name="troubleshooting"></a>

To execute the following commands, you will first need to be inside the pod of the Cluster Agent or the Node Agent.
You can use `kubectl exec -it <datadog-cluster-agent pod name> bash`
  
#### On the Datadog Cluster Agent side

To see what cluster level metadata is served by the Datadog Cluster Agent exec in the pod and run:

```
root@datadog-cluster-agent-8568545574-x9tc9:/# datadog-cluster-agent metamap

===============
Metadata Mapper
===============

Node detected: gke-test-default-pool-068cb9c0-sf1w

  - Namespace: kube-system
      - Pod: kube-dns-788979dc8f-hzbj5
        Services: [kube-dns]
      - Pod: kube-state-metrics-5587867c9f-xllnm
        Services: [kube-state-metrics]
      - Pod: kubernetes-dashboard-598d75cb96-5khmj
        Services: [kubernetes-dashboard]

Node detected: gke-test-default-pool-068cb9c0-wntj

  - Namespace: default
      - Pod: datadog-cluster-agent-8568545574-x9tc9
        Services: [datadog-custom-metrics-server dca]

  - Namespace: kube-system
      - Pod: heapster-v1.5.2-6d59ff54cf-g7q4h
        Services: [heapster]
      - Pod: kube-dns-788979dc8f-q9qkt
        Services: [kube-dns]
      - Pod: l7-default-backend-5d5b9874d5-b2lts
        Services: [default-http-backend]
      - Pod: metrics-server-v0.2.1-7486f5bd67-v827f
        Services: [metrics-server]
```

To verify that the Datadog Cluster Agent is being queried, look for:

```
root@datadog-cluster-agent-8568545574-x9tc9:/# tail -f /var/log/datadog/cluster-agent.log
2018-06-11 09:37:20 UTC | DEBUG | (metadata.go:40 in GetPodMetadataNames) | CacheKey: agent/KubernetesMetadataMapping/ip-192-168-226-77.ec2.internal, with 1 services
2018-06-11 09:37:20 UTC | DEBUG | (metadata.go:40 in GetPodMetadataNames) | CacheKey: agent/KubernetesMetadataMapping/ip-192-168-226-77.ec2.internal, with 1 services
```

If you are not collecting events properly, make sure to have those environment variables set to true:
- The leader election `DD_LEADER_ELECTION`
- The event collection `DD_COLLECT_KUBERNETES_EVENTS`

As well as the proper verbs listed in the RBAC (notably, `watch events`).

If you have enabled those, check the Leader Election status and the kube_apiserver check:

```

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

#### On the Node Agent side

You can check the status of the Datadog Cluster Agent while running the status command of the agent:
`agent status`

If the Datadog Cluster Agent is enabled and correctly configured, you should see:

```
[...]
 =====================
 Datadog Cluster Agent
 =====================
   - Datadog Cluster Agent endpoint detected: https://XXX.XXX.XXX.XXX:5005
   Successfully Connected to the Datadog Cluster Agent.
   - Running: {Major:1 Minor:0 Pre:xxx Meta:xxx Commit:xxxxx}
```

Make sure the Cluster Agent service was created before the agents' pods, so that the DNS is available in the environment variables:

```
root@datadog-agent-9d5bl:/# env | grep DATADOG_CLUSTER_AGENT | sort
DATADOG_CLUSTER_AGENT_SERVICE_PORT=5005
DATADOG_CLUSTER_AGENT_SERVICE_HOST=10.100.202.234
DATADOG_CLUSTER_AGENT_PORT_5005_TCP_PORT=5005
DATADOG_CLUSTER_AGENT_PORT=tcp://10.100.202.234:5005
DATADOG_CLUSTER_AGENT_PORT_5005_TCP=tcp://10.100.202.234:5005
DATADOG_CLUSTER_AGENTPORT_5005_TCP_PROTO=tcp
DATADOG_CLUSTER_AGENT_PORT_5005_TCP_ADDR=10.100.202.234

root@datadog-agent-9d5bl:/# echo ${DD_CLUSTER_AGENT_AUTH_TOKEN}
DD_CLUSTER_AGENT_AUTH_TOKEN=1234****9
```

Verify that the Node Agent is using the Datadog Cluster Agent as a tag provider:

```
root@datadog-agent-9d5bl:/# cat /var/log/datadog/agent.log | grep "metadata-collector"
2018-06-11 06:59:02 UTC | INFO | (tagger.go:151 in tryCollectors) | kube-metadata-collector tag collector successfully started
```

Or look for error logs, such as:

```
2018-06-10 08:03:02 UTC | ERROR | Could not initialise the communication with the Datadog Cluster Agent, falling back to local service mapping: [...]
```
