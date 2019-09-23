# Demo Setup

In order to use the `java-db-demo.yaml` application, you need to configure the `docker-registry-key` secret to allows
kubernetes to pull images from the private docker registry.


```bash
kubectl create secret generic docker-registry-key \
  --from-file=.dockerconfigjson=$HOME/.docker/config.json \
  --type=kubernetes.io/dockerconfigjson
```
