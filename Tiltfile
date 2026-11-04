# -*- mode: Python -*-

load('ext://ko', 'ko_build')

ko_build(
  'kind.local/patch-operator',
  '.',
  deps=['./main.go', './api', './controllers'],
)

allow_k8s_contexts(k8s_context())
k8s_yaml(kustomize('./config/local-development/tilt'))
k8s_resource('patch-operator-controller-manager',resource_deps=['patch-operator-compile'])