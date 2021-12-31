# Test

```shell
oc new-project test-patch-operator
oc create serviceaccount test -n test-patch-operator
#shortcut to get all permissions, we are not testing permissions, but the actual patching capability
oc adm policy add-cluster-role-to-user cluster-admin -z default -n test-patch-operator
```

## Simple patch

```shell
oc delete patch -n test-patch-operator --all
oc apply -f ./test/simple_patch.yaml -n test-patch-operator
```

## Complex patch

```shell
oc delete patch -n test-patch-operator --all
oc apply -f ./test/complex_patch.yaml -n test-patch-operator
```

## Field-level patch

```shell
oc delete patch -n test-patch-operator --all
oc apply -f ./test/field_patch.yaml -n test-patch-operator
```

## Multiple namespaced targets patch

```shell
oc delete patch -n test-patch-operator --all
oc apply -f ./test/multiple-namespaced-targets.yaml -n test-patch-operator
```

## Multiple cluster targets patch

```shell
oc delete patch -n test-patch-operator --all
oc apply -f ./test/multiple-cluster-targets.yaml -n test-patch-operator
```

## Test injection

```shell
oc delete patch -n test-patch-operator --all
oc apply -f ./test/inject-webhook.yaml
```

## Test simple injection

```shell
oc apply -f ./test/simple-injection.yaml -n test-patch-operator
```

## Test conmplex injection

```shell
oc apply -f ./test/complex-injection.yaml -n test-patch-operator
```