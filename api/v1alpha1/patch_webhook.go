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
	"context"
	"errors"
	"reflect"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var patchlog = logf.Log.WithName("patch-resource")

func (r *Patch) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(r).
		WithValidator(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-redhatcop-redhat-io-v1alpha1-patch,mutating=true,failurePolicy=fail,sideEffects=None,groups=redhatcop.redhat.io,resources=patches,verbs=create,versions=v1alpha1,name=mpatch.kb.io,admissionReviewVersions=v1

var _ admission.Defaulter[*Patch] = &Patch{}

// Default implements admission.Defaulter so a webhook will be registered for the type
func (r *Patch) Default(ctx context.Context, obj *Patch) error {
	patchlog.Info("default", "name", obj.Name)
	if !controllerutil.ContainsFinalizer(obj, PatchControllerFinalizerName) {
		controllerutil.AddFinalizer(obj, PatchControllerFinalizerName)
	}
	return nil
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-redhatcop-redhat-io-v1alpha1-patch,mutating=false,failurePolicy=fail,sideEffects=None,groups=redhatcop.redhat.io,resources=patches,verbs=update,versions=v1alpha1,name=vpatch.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*Patch] = &Patch{}

// ValidateCreate implements admission.Validator so a webhook will be registered for the type
func (r *Patch) ValidateCreate(ctx context.Context, obj *Patch) (admission.Warnings, error) {
	patchlog.Info("validate create", "name", obj.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil, nil
}

// ValidateUpdate implements admission.Validator so a webhook will be registered for the type
func (r *Patch) ValidateUpdate(ctx context.Context, old, obj *Patch) (admission.Warnings, error) {
	patchlog.Info("validate update", "name", obj.Name)

	if !reflect.DeepEqual(obj.Spec.ServiceAccountRef, old.Spec.ServiceAccountRef) {
		return nil, errors.New(".spec.serviceAccountRef is immutable after creation")
	}
	return nil, nil
}

// ValidateDelete implements admission.Validator so a webhook will be registered for the type
func (r *Patch) ValidateDelete(ctx context.Context, obj *Patch) (admission.Warnings, error) {
	patchlog.Info("validate delete", "name", obj.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
