
# Patch Operator

![build status](https://github.com/redhat-cop/patch-operator/workflows/push/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/redhat-cop/patch-operator)](https://goreportcard.com/report/github.com/redhat-cop/patch-operator)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/redhat-cop/patch-operator)
[![CRD Docs](https://img.shields.io/badge/CRD-Docs-brightgreen)](https://doc.crds.dev/github.com/redhat-cop/patch-operator)

The patch operator helps with defining patches in a declarative way. This operator has two main features:

1. [ability to patch an object at creation time via a mutating webhook](#creation-time-patch-injection)
2. [ability to enforce patches on one or more objects via a controller](#runtime-patch-enforcement)

## Index

- [Patch Operator](#patch-operator)
  - [Index](#index)
  - [Creation-time patch injection](#creation-time-patch-injection)
    - [Security Considerations](#security-considerations)
    - [Installing the creation time webhook](#installing-the-creation-time-webhook)
  - [Runtime patch enforcement](#runtime-patch-enforcement)
    - [Patch Controller Security Considerations](#patch-controller-security-considerations)
    - [Patch Controller Performance Considerations](#patch-controller-performance-considerations)
  - [Deploying the Operator](#deploying-the-operator)
    - [Multiarch Support](#multiarch-support)
    - [Deploying from OperatorHub](#deploying-from-operatorhub)
      - [Deploying from OperatorHub UI](#deploying-from-operatorhub-ui)
      - [Deploying from OperatorHub using CLI](#deploying-from-operatorhub-using-cli)
    - [Deploying with Helm](#deploying-with-helm)
  - [Metrics](#metrics)
    - [Testing metrics](#testing-metrics)
  - [Development](#development)
    - [Run the operator](#run-the-operator)
    - [Test Manually](#test-manually)
    - [Test helm chart locally](#test-helm-chart-locally)
  - [Building/Pushing the operator image](#buildingpushing-the-operator-image)
  - [Deploy to OLM via bundle](#deploy-to-olm-via-bundle)
  - [Releasing](#releasing)
    - [Cleaning up](#cleaning-up)

## Creation-time patch injection

Why apply a patch at creation time when you could directly create the correct object? The reason is that sometime the correct value depends on configuration set on the specific cluster in which the object is being deployed. For example, an ingress/route hostname might depend on the specific cluster. Consider the following example based on cert-manager:

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-issuer
spec:
  acme:
    server: 'https://acme-v02.api.letsencrypt.org/directory'
    email: {{ .Values.letsencrypt.email }}
    privateKeySecretRef:
      name: letsencrypt-staging
    solvers:  
    - dns01:
        route53:
          accessKeyID: << access_key >>
          secretAccessKeySecretRef:
            name: cert-manager-dns-credentials
            key: aws_secret_access_key
          region: << region >>
          hostedZoneID: << hosted_zone_id >>
```

In this example the fields: `<< access_key >>`, `<< region >>` and `<< hosted_zone_id >>` are dependent on the specific region in which the cluster is being deployed and in many cases they are discoverable from other configurations already present in the cluster. If you want to deploy the above Cluster Issuer object with a gitops approach, then there is no easy way to discover those values. The solution so far is to manually discover those values and create a different gitops configuration for each cluster. But consider if you could look up values at deploy time based on the cluster you are deploying to. Here is how this object might look:

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-issuer
  namespace: {{ .Release.Namespace }}
  annotations:
    "redhat-cop.redhat.io/patch": |
      spec:
        acme:
        - dns01:
            route53:
              accessKeyID: {{ (lookup "v1" "Secret" .metadata.namespace "cert-manager-dns-credentials").data.aws_access_key_id | b64dec }}
              secretAccessKeySecretRef:
                name: cert-manager-dns-credentials
                key: aws_secret_access_key
              region: {{ (lookup "config.openshift.io/v1" "Infrastructure" "" "cluster").status.platformStatus.aws.region }}
              hostedZoneID: {{ (lookup "config.openshift.io/v1" "DNS" "" "cluster").spec.publicZone.id }} 
spec:
  acme:
    server: 'https://acme-v02.api.letsencrypt.org/directory'
    email: {{ .Values.letsencrypt.email }}
    privateKeySecretRef:
      name: letsencrypt-staging
    solvers:  
    - dns01:
        route53:
          accessKeyID: << access_key >>
          secretAccessKeySecretRef:
            name: cert-manager-dns-credentials
            key: aws_secret_access_key
          region: << region >>
          hostedZoneID: << hosted_zone_id >>
```

The annotation specifies a patch that will be applied by a [MutatingWebhook](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/), as you can see the three values are now looked up from different configurations in the cluster.

Two annotations influences the behavior of this MutatingWebhook

1. "redhat-cop.redhat.io/patch" : this is the patch itself. The patch is evaluated as a template with the object itself as it's only parameter. The template is expressed in golang template notation and supports the same functions as helm template including the [lookup](https://helm.sh/docs/chart_template_guide/functions_and_pipelines/#using-the-lookup-function) function which plays a major role here. The patch must be expressed in yaml for readability. It will be converted to json by the webhook logic.
2. "redhat-cop.redhat.io/patch-type" : this is the type of json patch. The possible values are: `application/json-patch+json`, `application/merge-patch+json` and `application/strategic-merge-patch+json`. If this annotation is omitted it defaults to strategic merge.

### Security Considerations

The lookup function, if used by the template, is executed with a client which impersonates the user issuing the object creation/update request. This should prevent security permission leakage.

### Installing the creation time webhook

The creation time webhook is not installed by the operator. This is because there is no way to know which specific object type should be intercepted and intercepting all of the types would be too inefficient. It's up to the administrator then to install the webhook. Here is some guidance.

If you installed the operator via OLM, use the following webhook template:

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: patch-operator-inject
  annotations:
    service.beta.openshift.io/inject-cabundle: "true"
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    service:
      name: patch-operator-webhook-service
      namespace: patch-operator
      path: /inject
  failurePolicy: Fail
  name: patch-operator-inject.redhatcop.redhat.io
  rules:
  - << add your intercepted objects here >>
  sideEffects: None
```

If you installed the operator via the Helm chart and are using cert-manager, use the following webhook template:

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: patch-operator-inject
  annotations:
    cert-manager.io/inject-ca-from: '{{ .Release.Namespace }}/webhook-server-cert'
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    service:
      name: patch-operator-webhook-service
      namespace: patch-operator
      path: /inject
  failurePolicy: Fail
  name: patch-operator-inject.redhatcop.redhat.io
  rules:
  - << add your intercepted objects here >>
  sideEffects: None
```  

You should need to enable the webhook only for `CREATE` operations. So for example to enable the webhook on configmaps:

```yaml
  rules:
  - apiGroups:
    - ""
    apiVersions:
    - v1
    operations:
    - CREATE
    resources:
    - configmaps
```

## Runtime patch enforcement

There are situations when we need to patch pre-existing objects. Again this is a use case that is hard to model with gitops operators which will work only on object that they own. Especially with sophisticated Kubernetes distributions, it is not uncommon that a Kubernetes instance, at installation time, is configured with some default settings. Changing those configurations means patching those objects. For example, let's take the case of OpenShift Oauth configuration. This object is present by default and it is expected to be patched with any newly enabled authentication mechanism. This is how it looks like after installation:

```yaml
apiVersion: config.openshift.io/v1
kind: OAuth
metadata:
  name: cluster
  ownerReferences:
    - apiVersion: config.openshift.io/v1
      kind: ClusterVersion
      name: version
      uid: 9a9d450b-3076-4e30-ac05-a889d6341fc3
  resourceVersion: '20405124'
spec: []
```

If we need to patch it we can use the patch controller and the `Patch` object as follows:

```yaml
apiVersion: redhatcop.redhat.io/v1alpha1
kind: Patch
metadata:
  name: gitlab-ocp-oauth-provider
  namespace: openshift-config
spec:
  serviceAccountRef:
    name: default
  patches:
    gitlab-ocp-oauth-provider:
      targetObjectRef:
        apiVersion: config.openshift.io/v1
        kind: OAuth
        name: cluster
      patchTemplate: |
        spec:
          identityProviders:
          - name: my-github 
            mappingMethod: claim 
            type: GitHub
            github:
              clientID: "{{ (index . 1).data.client_id | b64dec }}" 
              clientSecret: 
                name: ocp-github-app-credentials
              organizations: 
              - my-org
              teams: []            
      patchType: application/merge-patch+json
      sourceObjectRefs:
      - apiVersion: v1
        kind: Secret
        name: ocp-github-app-credentials
        namespace: openshift-config
```

This will cause the OAuth object to be patched and the patch to be enforced. That means that if anything changes on the secret that we use a parameter (which may be rotated) or in Oauth object itself, the patch will be reapplied. In this case we are adding a gitlab authentication provider.

A `patch` has the following fields:

`targetObjectRef` this refers to the object(s) receiving the patch. Mutliple object can be selected based on the following rules:

| Namespaced Type | Namespace | Name | Selection type |
| --- | --- | --- | --- |
| yes | null | null | multiple selection across namespaces |
| yes | null | not null | multiple selection across namespaces where the name corresponds to the passed name |
| yes | not null | null | multiple selection within a namespace |
| yes | not null | not nul | single selection |
| no | N/A | null | multiple selection  |
| no | N/A | not null | single selection |

Selection can be further narrowed down by filtering by labels and/or annotations using the `labelSelector` and `annotationSelector` fields. The patch will be applied to all of the selected instances.

`sourceObjectRefs` these are the objects that will be watched and become part of the parameters of the patch template. Name and Namespace of sourceRefObjects are interpreted as golang templates with the current target instance and the only parameter. This allows to select different source object for each target object.

So, for example, with this patch:

```yaml
apiVersion: redhatcop.redhat.io/v1alpha1
kind: Patch
metadata:
  name: multiple-namespaced-targets-patch
spec:
  serviceAccountRef:
    name: default
  patches:
     multiple-namespaced-targets-patch:
      targetObjectRef:
        apiVersion: v1
        kind: ServiceAccount
        name: deployer
      patchTemplate: |
        metadata:
          annotations:
            {{ (index . 1).metadata.uid }}: {{ (index . 1) }}
      patchType: application/strategic-merge-patch+json
      sourceObjectRefs:
      - apiVersion: v1
        kind: ServiceAccount
        name: default
        namespace: "{{ .metadata.namespace }}"
        fieldPath: $.metadata.uid
```

The `deployer` service accounts from all namespaces are selected as target of this patch, each patch template will receive a different parameter and that is the `default` service account of the same namespace as the namespace of the `deployer` service account being processed.

`sourceObjectRefs` also have the `fieldPath` field which can contain a jsonpath expression. If a value is passed the jsonpath expression will be calculate for the current source object and the result will be passed as parameter of the template.

`patchTemplate` This is the the template that will be evaluated. The result must be a valid patch compatible with the requested type and expressed in yaml for readability. The parameters passed to the template are the target object and then the all of the source object. So if you want to refer to the target object in the template you can use this expression `(index . 0)`. Higher indexes refer to the sourceObjectRef array. The template is expressed in golang template notation and supports the same functions as helm template.

`patchType` is the type of the json patch. The possible values are: `application/json-patch+json`, `application/merge-patch+json` and `application/strategic-merge-patch+json`. If this annotation is omitted it defaults to strategic merge.

### Patch Controller Security Considerations

The patch enforcement enacted by the patch controller is executed with a client which uses the service account referenced by the `serviceAccountRef` field. So before a patch object can actually work an administrator must have granted the needed permissions to a service account in the same namespace. The `serviceAccountRef` will default to the `default` service account if not specified.

### Patch Controller Performance Considerations

The patch controller will create a controller-manager and per `Patch` object and a reconciler for each of the `PatchSpec` defined in the array on patches in the `Patch` object.
These reconcilers share the same cached client. In order to be able to watch changes on target and source objects of a `PatchSpec`, all of the target and source object type instances will be cached by the client. This is a normal behavior of a controller-manager client, but it implies that if you create patches on object types that have many instances in etcd (Secrets, ServiceAccounts, Namespaces for example), the patch operator instance will require a significant amount of memory. A way to contain this issue is to try to aggregate together `PatchSpec` that deal with the same object types. This will cause those object type instances to cached only once.

## Deploying the Operator

This is a cluster-level operator that you can deploy in any namespace, `patch-operator` is recommended.

It is recommended to deploy this operator via [`OperatorHub`](https://operatorhub.io/), but you can also deploy it using [`Helm`](https://helm.sh/).

### Multiarch Support

| Arch  | Support  |
|:-:|:-:|
| amd64  | ✅ |
| arm64  | ✅  |
| ppc64le  | ✅  |
| s390x  | ❌  |

### Deploying from OperatorHub

> **Note**: This operator supports being installed disconnected environments

If you want to utilize the Operator Lifecycle Manager (OLM) to install this operator, you can do so in two ways: from the UI or the CLI.

#### Deploying from OperatorHub UI

- If you would like to launch this operator from the UI, you'll need to navigate to the OperatorHub tab in the console. Before starting, make sure you've created the namespace that you want to install this operator to with the following:

```shell
oc new-project patch-operator
```

- Once there, you can search for this operator by name: `vault config operator`. This will then return an item for our operator and you can select it to get started. Once you've arrived here, you'll be presented with an option to install, which will begin the process.
- After clicking the install button, you can then select the namespace that you would like to install this to as well as the installation strategy you would like to proceed with (`Automatic` or `Manual`).
- Once you've made your selection, you can select `Subscribe` and the installation will begin. After a few moments you can go ahead and check your namespace and you should see the operator running.

#### Deploying from OperatorHub using CLI

If you'd like to launch this operator from the command line, you can use the manifests contained in this repository by running the following:

oc new-project patch-operator

```shell
oc apply -f config/operatorhub -n patch-operator
```

This will create the appropriate OperatorGroup and Subscription and will trigger OLM to launch the operator in the specified namespace.

### Deploying with Helm

Here are the instructions to install the latest release with Helm.

```shell
oc new-project patch-operator
helm repo add patch-operator https://redhat-cop.github.io/patch-operator
helm repo update
helm install patch-operator patch-operator/patch-operator
```

This can later be updated with the following commands:

```shell
helm repo update
helm upgrade patch-operator patch-operator/patch-operator
```

## Metrics

Prometheus compatible metrics are exposed by the Operator and can be integrated into OpenShift's default cluster monitoring. To enable OpenShift cluster monitoring, label the namespace the operator is deployed in with the label `openshift.io/cluster-monitoring="true"`.

```shell
oc label namespace <namespace> openshift.io/cluster-monitoring="true"
```

### Testing metrics

```sh
export operatorNamespace=patch-operator-local # or patch-operator
oc label namespace ${operatorNamespace} openshift.io/cluster-monitoring="true"
oc rsh -n openshift-monitoring -c prometheus prometheus-k8s-0 /bin/bash
export operatorNamespace=patch-operator-local # or patch-operator
curl -v -s -k -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" https://patch-operator-controller-manager-metrics.${operatorNamespace}.svc.cluster.local:8443/metrics
exit
```

## Development

### Run the operator

```shell
export repo=raffaelespazzoli
docker login quay.io/$repo
oc new-project patch-operator
oc project patch-operator
envsubst < config/local-development/tilt/env-replace-image.yaml > config/local-development/tilt/replace-image.yaml
tilt up
```

### Test Manually

see [here](./test/readme.md)

### Test helm chart locally

Define an image and tag. For example...

```shell
export imageRepository="quay.io/redhat-cop/patch-operator"
export imageTag="$(git -c 'versionsort.suffix=-' ls-remote --exit-code --refs --sort='version:refname' --tags https://github.com/redhat-cop/patch-operator.git '*.*.*' | tail --lines=1 | cut --delimiter='/' --fields=3)"
```

Deploy chart...

```shell
make helmchart IMG=${imageRepository} VERSION=${imageTag}
helm upgrade -i patch-operator-local charts/patch-operator -n patch-operator-local --create-namespace
```

Delete...

```shell
helm delete patch-operator-local -n patch-operator-local
kubectl delete -f charts/patch-operator/crds/crds.yaml
```

## Building/Pushing the operator image

```shell
export repo=raffaelespazzoli #replace with yours
docker login quay.io/$repo
make docker-build IMG=quay.io/$repo/patch-operator:latest
make docker-push IMG=quay.io/$repo/patch-operator:latest
```

## Deploy to OLM via bundle

```shell
make manifests
make bundle IMG=quay.io/$repo/patch-operator:latest
operator-sdk bundle validate ./bundle --select-optional name=operatorhub
make bundle-build BUNDLE_IMG=quay.io/$repo/patch-operator-bundle:latest
docker push quay.io/$repo/patch-operator-bundle:latest
operator-sdk bundle validate quay.io/$repo/patch-operator-bundle:latest --select-optional name=operatorhub
oc new-project patch-operator
oc label namespace patch-operator openshift.io/cluster-monitoring="true"
operator-sdk cleanup patch-operator -n patch-operator
operator-sdk run bundle --install-mode AllNamespaces -n patch-operator quay.io/$repo/patch-operator-bundle:latest
```

## Releasing

```shell
git tag -a "<tagname>" -m "<commit message>"
git push upstream <tagname>
```

If you need to remove a release:

```shell
git tag -d <tagname>
git push upstream --delete <tagname>
```

If you need to "move" a release to the current main

```shell
git tag -f <tagname>
git push upstream -f <tagname>
```

### Cleaning up

```shell
operator-sdk cleanup patch-operator -n patch-operator
oc delete operatorgroup operator-sdk-og
oc delete catalogsource patch-operator-catalog
```
