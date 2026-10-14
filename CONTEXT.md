# Kubectl Status

A kubectl plugin that presents the conditions of selected Kubernetes resources.

## Language

**Selected resource**:
A Kubernetes resource returned by the user's resource selection, including custom resources.

**Condition row**:
One entry from a selected resource's `status.conditions` list, shown in that resource's condition table.

**Empty result**:
A resource selection that finds no resource instances. It is distinct from a selection that refers to an unknown resource type.
