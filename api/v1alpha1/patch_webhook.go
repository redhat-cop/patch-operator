/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"errors"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var patchlog = logf.Log.WithName("patch-resource")

func (r *Patch) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-redhatcop-redhat-io-v1alpha1-patch,mutating=true,failurePolicy=fail,sideEffects=None,groups=redhatcop.redhat.io,resources=patches,verbs=create,versions=v1alpha1,name=mpatch.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Patch{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Patch) Default() {
	patchlog.Info("default", "name", r.Name)
	if !controllerutil.ContainsFinalizer(r, PatchControllerFinalizerName) {
		controllerutil.AddFinalizer(r, PatchControllerFinalizerName)
	}
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-redhatcop-redhat-io-v1alpha1-patch,mutating=false,failurePolicy=fail,sideEffects=None,groups=redhatcop.redhat.io,resources=patches,verbs=update,versions=v1alpha1,name=vpatch.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Patch{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Patch) ValidateCreate() error {
	patchlog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Patch) ValidateUpdate(old runtime.Object) error {
	patchlog.Info("validate update", "name", r.Name)

	if !reflect.DeepEqual(r.Spec.ServiceAccountRef, old.(*Patch).Spec.ServiceAccountRef) {
		return errors.New(".spec.serviceAccountRef is immutable after creation")
	}
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Patch) ValidateDelete() error {
	patchlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
