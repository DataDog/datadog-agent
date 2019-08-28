# Demo Setup

In order to use the `java-db-demo.yaml` application, you need to configure the `ecr-reg-key` secret to allows
kubernetes to pull images from the StackState ecr registry.


```bash
kubectl create secret generic ecr-reg-key \
  --from-file=.dockerconfigjson=$HOME/.docker/config.json \
  --type=kubernetes.io/dockerconfigjson
```
