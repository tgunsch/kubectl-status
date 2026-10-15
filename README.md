# kubectl-status

Show the `status.conditions` of Kubernetes resources in a table, newest `lastTransitionTime` first.

## Build and install

Requires Go and `kubectl` configured for a Kubernetes cluster. Build a binary named `kubectl-status` and put it on your `PATH`:

```sh
go build -o kubectl-status .
install kubectl-status $HOME/bin/kubectl-status
```

## Usage

```sh
kubectl status deployment/my-app
kubectl status deployments -n staging
kubectl status deployments,services -A
kubectl status deployment/my-app -s Ready
kubectl status jobs --summary-condition Succeeded
```

The plugin uses the namespace in your current kubectl context by default. `-n`/`--namespace` selects one namespace; `-A`/`--all-namespaces` selects all namespaces. The first version accepts one `type/name` or one comma-separated list of resource types. Label selectors are not supported.

With `-s CONDITION`/`--summary-condition CONDITION`, each resource shows only conditions of that type with status `True` when at least one is present. Otherwise, all its conditions are shown. The table format and ordering are the same as without summary mode.

Each resource is headed by `namespace/type/name` (or `type/name` for cluster-scoped resources). Resources without conditions show `No conditions`; selections with no matches show `No resources found`. Unknown resource types and other kubectl errors are printed to stderr and result in a nonzero exit status. Conditions without `lastTransitionTime` appear last, with `-` for the timestamp; ties retain their original order. Messages with newlines or tabs are escaped to keep each condition on one row.
