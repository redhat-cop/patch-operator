apiVersion: v1
name: patch-operator
version: ${version}
appVersion: ${version}
description: Helm chart that deploys patch-operator
keywords:
  - volume
  - storage
  - csi
  - expansion
  - monitoring
sources:
  - https://github.com/redhat-cop/patch-operator
engine: gotpl