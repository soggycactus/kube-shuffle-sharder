# kube-shuffle-sharder

kube-shuffle-sharder is an operator that implements [shuffle sharding](https://aws.amazon.com/builders-library/workload-isolation-using-shuffle-sharding/) for Kubernetes node groups. With shuffle sharding, multi-tenant clusters can take advantage of combinatorial math to increase isolation between tenants while continuing to efficiently share infrastructure. If a noisy tenant or rogue infrastructure operation takes down a node group, shuffle sharding ensures that tenants remain online & experience a partial service degradation in the worst case scenario.

kube-shuffle-sharder works by injecting a `requiredDuringSchedulingIgnoredDuringExecution` [node affinity](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#node-affinity) on pods to restrict their scheduling to a specific subset of node groups. The informer will watch & auto-discover all node groups by inspecting a label (`kube-shuffle-sharder.io/node-group` by default) on each node. The mutating webhook intercepts pod creation requests & inspects a label (`kube-shuffle-sharder.io/tenant` by default) to identify which tenant the pod belongs to - if a `ShuffleShard` resource already exists for this tenant, the webhook will retrieve the shard value & inject a node affinity to the pod. 

If no shuffle shard exists for the tenant, the webhook will create one - ensuring all following pod creation requests are routed to the same shard. For example, the following shuffle shard:

```
apiVersion: kube-shuffle-sharder.io/v1
kind: ShuffleShard
metadata:
  name: tenant-b
spec:
  nodeGroups:
  - group-c
  - group-a
  tenant: tenant-b
```

will result in the following pod spec:

```
apiVersion: v1
kind: Pod
metadata:
  labels:
    kube-shuffle-sharder.io/tenant: tenant-b
  name: tenant-b
  namespace: default
spec:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: kube-shuffle-sharder.io/node-group
            operator: In
            values:
            - group-c
            - group-a
  containers:
  - image: nginx:1.14.2
    imagePullPolicy: IfNotPresent
    name: nginx
```

`ShuffleShards` are immutable - fixed assignment is required to implement shuffle sharding; any attempt to automatically failover tenants to different shards breaks the isolation provided by shuffle sharding. Since the shards are only evaluated during scheduling, you can delete an existing shard & re-deploy a tenant's pods to move them to a different shard if desired.

Shuffle sharding is similar to the [n choose k](https://www.hackmath.net/en/calculator/n-choose-k) problem; given a fixed number of node groups & a desired shard size, there is a finite number of shuffle shards possible. Once all shuffle shards have been allocated, pod creation requests will fail with the error `no shards available`.

To monitor for shard usage, you can scrape the `/metrics` endpoint for `kube_shuffle_sharder_num_shuffle_shards_possible` and `kube_shuffle_sharder_num_shuffle_shards_used`, as well as additional latency metrics.

To use kube-shuffle-sharder, all you need to do is:

1. Create a label on your nodes that identify which node-group they are a part of
2. Create a label on your pods that identify which tenant the pod belongs to
3. Install kube-shuffle-sharder in your cluster

Installing kube-shuffle-sharder is made simple through the Helm chart provided in this repository. You can specify the `nodeGroupAutoDiscoveryLabel`, `tenantLabel`, and the `numNodeGroups` per shuffle shard. Additionally, you can configure a `webhookSecretName` and `webhookCaBundle` to use your own certificate - otherwise the Helm chart will generate one for you.

## License

Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

