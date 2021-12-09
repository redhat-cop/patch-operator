package controllers

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"text/template"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/redhat-cop/operator-utils/pkg/util/discoveryclient"
	utilstemplate "github.com/redhat-cop/operator-utils/pkg/util/templates"
	v1authn "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/yaml"
)

const patchKey string = "redhat-cop.redhat.io/patch"

type PatchType string

// allowed values one of "application/json-patch+json"'"application/merge-patch+json","application/strategic-merge-patch+json".  Default "application/strategic-merge-patch+json"
const patchTypeAnnotation string = "redhat-cop.redhat.io/patch-type"

const (
	jsonPatch           PatchType = "application/json-patch+json"
	mergePatch          PatchType = "application/merge-patch+json"
	strategicMergePatch PatchType = "application/strategic-merge-patch+json"
)

var createTimePatchLog = logf.Log.WithName("create-time-patch-webhook")

// podAnnotator annotates Pods
// +kubebuilder:object:generate:=false
type PatchInjector struct {
	client     client.Client
	restConfig *rest.Config
	decoder    *admission.Decoder
	crr        *CustomResourceDefinitionReconciler
}

func NewPatchInjector(client client.Client, restConfig *rest.Config, customResourceDefinitionReconciler *CustomResourceDefinitionReconciler) *PatchInjector {
	return &PatchInjector{
		client:     client,
		restConfig: restConfig,
		crr:        customResourceDefinitionReconciler,
	}
}

// podAnnotator adds an annotation to every incoming pods.
func (a *PatchInjector) Handle(ctx context.Context, req admission.Request) admission.Response {
	ctx = context.WithValue(ctx, "restConfig", a.restConfig)
	ctx = log.IntoContext(ctx, createTimePatchLog)
	obj := &unstructured.Unstructured{}

	err := a.decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	for key := range obj.GetAnnotations() {
		if key == patchKey {
			//compute the template

			templ, err := template.New(obj.GetAnnotations()[patchKey]).Funcs(a.advancedTemplateFuncMapWithImpersonation(ctx, &req.UserInfo)).Parse(obj.GetAnnotations()[patchKey])
			if err != nil {
				createTimePatchLog.Error(err, "unable to parse ", "template", obj.GetAnnotations()[patchKey])
				return admission.Errored(http.StatusInternalServerError, err)
			}

			var b bytes.Buffer
			err = templ.Execute(&b, obj)
			if err != nil {
				createTimePatchLog.Error(err, "unable to process ", "template ", templ, "parameters", obj)
				return admission.Errored(http.StatusInternalServerError, err)
			}

			bb, err := yaml.YAMLToJSON(b.Bytes())

			if err != nil {
				createTimePatchLog.Error(err, "unable to convert to json", "processed template", b.String())
				return admission.Errored(http.StatusInternalServerError, err)
			}
			patchType, ok := obj.GetAnnotations()[patchTypeAnnotation]
			if !ok {
				patchType = "application/strategic-merge-patch+json"
			}

			switch PatchType(patchType) {
			case jsonPatch:
				{
					patch, err := jsonpatch.DecodePatch(bb)
					if err != nil {
						createTimePatchLog.Error(err, "unable to decode", "jsonpatch", string(bb))
						return admission.Errored(http.StatusInternalServerError, err)
					}

					patchedObject, err := patch.Apply(req.Object.Raw)
					if err != nil {
						createTimePatchLog.Error(err, "unable patch object", "original", req.Object.Raw, "patch", string(bb))
						return admission.Errored(http.StatusInternalServerError, err)
					}
					return admission.PatchResponseFromRaw(req.Object.Raw, patchedObject)
				}
			case mergePatch:
				{
					patchedObject, err := jsonpatch.MergePatch(req.Object.Raw, bb)
					if err != nil {
						createTimePatchLog.Error(err, "unable patch object", "original", req.Object.Raw, "patch", string(bb))
						return admission.Errored(http.StatusInternalServerError, err)
					}
					return admission.PatchResponseFromRaw(req.Object.Raw, patchedObject)
				}
			case strategicMergePatch:
				{
					patchMeta, err := a.getPatchMeta(ctx, obj)
					if err != nil {
						createTimePatchLog.Error(err, "unable to get patchMeta", "for object", obj)
						return admission.Errored(http.StatusInternalServerError, err)
					}
					patchedObject, err := strategicpatch.StrategicMergePatchUsingLookupPatchMeta(req.Object.Raw, bb, patchMeta)
					if err != nil {
						createTimePatchLog.Error(err, "unable to get patch", "object", obj, "with patch", bb, "patch type", patchType)
						return admission.Errored(http.StatusInternalServerError, err)
					}
					return admission.PatchResponseFromRaw(req.Object.Raw, patchedObject)
				}
			default:
				{
					err := errors.New("unsupported patch type" + patchType)
					return admission.Errored(http.StatusInternalServerError, err)
				}
			}
		}
	}
	return admission.Allowed("no changes")
}

func getModelName(apiresource *metav1.APIResource) string {
	var group, version string
	if apiresource.Group == "" {
		group = "io.k8s.api.core"
	} else {
		if !strings.Contains(apiresource.Group, ".") {
			group = "io.k8s.api." + apiresource.Group
		} else {
			group = apiresource.Group
		}
	}

	if apiresource.Version == "" {
		version = "v1"
	} else {
		version = apiresource.Version
	}

	return group + "." + version + "." + apiresource.Kind
}

func (a *PatchInjector) getPatchMeta(context context.Context, obj *unstructured.Unstructured) (strategicpatch.LookupPatchMeta, error) {
	log := log.FromContext(context)
	resource, found, err := discoveryclient.GetAPIResourceForGVK(context, obj.GroupVersionKind())
	if err != nil {
		log.Error(err, "unable to find resource from", "GVK", obj.GroupVersionKind())
		return nil, err
	}
	if !found {
		return nil, errors.New("GVK not found: " + obj.GroupVersionKind().String())
	}
	openapiModels := a.crr.GetModels()
	log.V(1).Info("getPatchMeta", "getModelName(resource)", getModelName(resource))
	openapiSchema := openapiModels.LookupModel(getModelName(resource))
	return strategicpatch.NewPatchMetaFromOpenAPI(openapiSchema), nil
}

// InjectDecoder injects the decoder.
func (a *PatchInjector) InjectDecoder(d *admission.Decoder) error {
	a.decoder = d
	return nil
}

func (a *PatchInjector) advancedTemplateFuncMapWithImpersonation(ctx context.Context, userInfo *v1authn.UserInfo) template.FuncMap {
	rc := rest.CopyConfig(a.restConfig)
	rc.Impersonate.UserName = userInfo.Username
	rc.Impersonate.Groups = userInfo.Groups
	extra := map[string][]string{}
	for k := range userInfo.Extra {
		extra[k] = userInfo.Extra[k]
	}
	rc.Impersonate.Extra = extra
	funcs := utilstemplate.AdvancedTemplateFuncMap(rc, createTimePatchLog)
	return funcs
}
