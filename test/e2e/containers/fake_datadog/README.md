# fake_datadog


Expose the needed API to make the agent submit payloads.


#### API

Prefer using mongo.

Get all series:
```bash
curl ${SERVICE_IP}/records/series | jq .
```

Get all check_run:
```bash
curl ${SERVICE_IP}/records/check_run | jq .
```

Get all intake:
```bash
curl ${SERVICE_IP}/records/intake | jq .
```

#### MongoDB

Explore:
```bash
docker run --rm -it --net=host mongo mongo ${SERVICE_IP}/datadog
```
```bash
apt-get install -yqq mongodb-clients && mongo ${SERVICE_IP}/datadog
```
```bash
> show collections
check_run
intake
series

```

#### Find

Find a metric:
```text
> db.series.findOne()

{
	"_id" : ObjectId("5ab3e567cd9a72000912abad"),
	"metric" : "datadog.agent.running",
	"points" : [
		[
			1521739111,
			1
		]
	],
	"tags" : null,
	"host" : "haf",
	"type" : "gauge",
	"interval" : 0,
	"source_type_name" : "System"
}
```

Find a metric by metric name:
```text
db.series.findOne({metric: "kubernetes.network.tx_errors"})

{
	"_id" : ObjectId("5ab4cca8c914b50008c10615"),
	"metric" : "kubernetes.network.tx_errors",
	"points" : [
		[
			1521798304,
			0
		]
	],
	"tags" : [
		"kube_deployment:workflow-controller",
		"kube_namespace:kube-system",
		"kube_replica_set:workflow-controller-58bbf49865",
		"pod_name:workflow-controller-58bbf49865-55xdz"
	],
	"host" : "v1704",
	"type" : "gauge",
	"interval" : 0,
	"source_type_name" : "System"
}
```

Advanced find:
```js
db.series.find({
    metric: "kubernetes.cpu.usage.total",
    tags: { $all: ["kube_namespace:kube-system", "pod_name:kube-controller-manager"] }
}, {_id: 0}) // .count()
```

#### Aggregation pipeline

Aggregate all tags for a metric:
```js
db.series.aggregate([
    { $match: { metric: "kubernetes.cpu.usage.total"} },
    { $project: {tags: 1} },
    { $unwind: "$tags" },
    { $group: {_id: "allTags",  tags: {$addToSet:  "$tags" } } }
])
```

Aggregate all tags for a metric regex:
```js
db.series.aggregate([
    { $match: { metric: {$regex: "kubernetes*"} } },
    { $project: {tags: 1} },
    { $unwind: "$tags" },
    { $group: {_id: "allTags",  tags: {$addToSet:  "$tags" } } }
])
```

Aggregate all tags for each metric matched by a regex:
```js
db.series.aggregate([
    { $match: { metric: {$regex: "kubernetes*"} } },
    { $project: { metric: 1, tags: 1 } },
    { $unwind: "$tags" },
    { $group: {_id: "$metric",  tags: {$addToSet:  "$tags" } } }
])
```

Aggregate all metrics from a tag:
```js
db.series.aggregate([
    { $match: { tags: "kube_deployment:fake-app-datadog"} },
    { $group: { _id: "kube_deployment:fake-app-datadog", metrics: { $addToSet: "$metric" } } }
])
```

Aggregate all metrics from tags ($or || $and):
```js
db.series.aggregate([
    { $match: { $or: [
        {tags: "kube_deployment:fake-app-datadog"},
        {tags: "kube_service:fake-app-datadog"}
    ] } },
    { $group: { _id: "metricsToTags", metrics: { $addToSet: "$metric" } } }
])
```

Aggregate a metric and a tag as timeseries:
```js
db.series.aggregate([
	{ $match: { tags: "kube_deployment:dd", metric: "kubernetes.cpu.usage.total"} },
	{ $unwind: "$points" },
	{ $project: {
		_id: { $arrayElemAt: [ "$points", 0 ] },
		value: { $arrayElemAt: [ "$points", 1 ] },
		tags: "$tags"
		}
	},
	{ $sort: { _id: 1 } }
])
```

Count tag occurrences on a given metric:
```js
db.series.aggregate([
	{ $match: { metric: "kubernetes.filesystem.usage", tags: { $all: ["pod_name:fake-app-datadog-7cfb79db4d-dd4jr"] } } },
	{ $project: {tags: 1} },
	{ $unwind: "$tags" },
	{ $group: {_id: "$tags", count: { $sum: 1 } } },
	{ $sort: {count: -1} }
])
```

#### Use standalone

This tool can be used as a debug proxy to inspect agent payloads. Here is how to do it for Kubernetes.

- run the following from within this folder:

```console
docker build -t fake-datadog:latest .
docker tag fake-datadog:latest <YOUR_REPO/IMAGE:TAG>
docker push <YOUR_REPO/IMAGE:TAG>
# replace <YOUR_REPO/IMAGE:TAG> in fake-datadog.yaml before running the next command
kubectl apply -f fake-datadog.yaml
```

- edit your Datadog Agent Daemonset to use the service deployed above as the Datadog API. Be aware that each agent has its own intake - configuring `DD_DD_URL` doesn't cover the logs agent for example.

```yaml
...
  env:
    ...
    - name: DD_DD_URL
      # if you deployed the service & deployment in a separate namespace, add `.<NAMESPACE>.svc.cluster.local
      value: "http://fake-datadog"
```
