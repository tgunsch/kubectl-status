# kubectl-status

Show the `status.conditions` of Kubernetes resources in a table, newest `lastTransitionTime` first.

## See it in action

For example, a deployment that's still rolling out might look like this (illustrative output):

```text
$ kubectl status deployment/checkout -n staging
staging/deployment/checkout
TYPE         STATUS  REASON                      MESSAGE                           LAST TRANSITION TIME
Progressing  True    NewReplicaSetCreated        Started rolling out new replicas  2026-10-15T10:24:00Z
Available    False   MinimumReplicasUnavailable  Waiting for healthy replicas      2026-10-15T10:21:00Z
```

The latest transition appears first, so you can see both that the rollout is progressing and why the deployment isn't available yet. For several resources, each gets its own heading and condition table.

For multiple deployments, an output with a summary for the successful ones and a full table for the failing ones would look like this:

```text
$ kubectl status deployments -n staging -s "Available"
staging/deployment/cart
TYPE       STATUS  REASON                    MESSAGE                               LAST TRANSITION TIME
Available  True    MinimumReplicasAvailable  Deployment has minimum availability.  2026-10-15T10:21:55Z

staging/deployment/checkout
TYPE         STATUS  REASON                      MESSAGE                           LAST TRANSITION TIME
Available    False   MinimumReplicasUnavailable  Waiting for healthy replicas      2026-10-15T10:21:00Z
Progressing  True    NewReplicaSetCreated        Started rolling out new replicas  2026-10-15T10:24:00Z

staging/deployment/payments
TYPE       STATUS  REASON                    MESSAGE                               LAST TRANSITION TIME
Available  True    MinimumReplicasAvailable  Deployment has minimum availability.  2026-10-15T10:21:55Z
```


## Build and install

Requires Go and `kubectl` configured for a Kubernetes cluster. Build a binary named `kubectl-status` and put it on your `PATH`:

```sh
go build -o kubectl-status .
install kubectl-status $HOME/bin/kubectl-status
```

## Usage

```sh
# status of a single deployment
kubectl status deployment/my-app

# status of all deployments in the staging namespace
kubectl status deployments -n staging

# status of all deployments and services in all namespaces
kubectl status deployments,services -A

# status of a single deployment. 
# If the deployment is ready, only the "Ready" condition is shown, otherwise all conditions are shown.
kubectl status deployment/my-app -s Ready

# status of all jobs in the current namespace
# If the job is complete, only the "Succeeded" condition is shown, otherwise all conditions are shown.
kubectl status jobs --summary-condition Succeeded
```

The plugin uses the namespace in your current kubectl context by default. `-n`/`--namespace` selects one namespace; `-A`/`--all-namespaces` selects all namespaces. The first version accepts one `type/name` or one comma-separated list of resource types. Label selectors are not supported.

With `-s CONDITION`/`--summary-condition CONDITION`, each resource shows only conditions of that type with status `True` when at least one is present. Otherwise, all its conditions are shown. The table format and ordering are the same as without summary mode.

Each resource is headed by `namespace/type/name` (or `type/name` for cluster-scoped resources). Resources without conditions show `No conditions`; selections with no matches show `No resources found`. Unknown resource types and other kubectl errors are printed to stderr and result in a nonzero exit status. Conditions without `lastTransitionTime` appear last, with `-` for the timestamp; ties retain their original order. Messages with newlines or tabs are escaped to keep each condition on one row.
