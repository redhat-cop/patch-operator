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

package controllers

import (
	"context"
	errs "errors"

	"github.com/redhat-cop/operator-utils/pkg/util"
	"github.com/redhat-cop/operator-utils/pkg/util/apis"
	"github.com/redhat-cop/operator-utils/pkg/util/lockedresourcecontroller"
	"github.com/redhat-cop/operator-utils/pkg/util/lockedresourcecontroller/lockedpatch"
	redhatcopv1alpha1 "github.com/redhat-cop/patch-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// PatchReconciler reconciles a Patch object
type PatchReconciler struct {
	lockedresourcecontroller.EnforcingReconciler
}

//+kubebuilder:rbac:groups=redhatcop.redhat.io,resources=patches,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=redhatcop.redhat.io,resources=patches/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=redhatcop.redhat.io,resources=patches/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;patch
//+kubebuilder:rbac:groups="",resources=serviceaccounts;secrets,verbs=get;list;watch

// needed by the patch webhook
//+kubebuilder:rbac:groups="*",resources="*",verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=users;groups;serviceaccounts,verbs=impersonate
//+kubebuilder:rbac:groups="authentication.k8s.io",resources=*,verbs=impersonate

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Patch object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *PatchReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	rlog := log.FromContext(ctx).WithName(req.Namespace + "-" + req.Name)
	ctx = log.IntoContext(ctx, rlog)
	// Fetch the ResourceLocker instance
	instance := &redhatcopv1alpha1.Patch{}
	err := r.GetClient().Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if util.IsBeingDeleted(instance) {
		if !controllerutil.ContainsFinalizer(instance, redhatcopv1alpha1.PatchControllerFinalizerName) {
			return reconcile.Result{}, nil
		}
		err := r.manageCleanUpLogic(ctx, instance)
		if err != nil {
			rlog.Error(err, "unable to delete instance", "instance", instance)
			return r.ManageError(ctx, instance, err)
		}
		controllerutil.RemoveFinalizer(instance, redhatcopv1alpha1.PatchControllerFinalizerName)
		err = r.GetClient().Update(ctx, instance)
		if err != nil {
			rlog.Error(err, "unable to update instance", "instance", instance)
			return r.ManageError(ctx, instance, err)
		}
		return reconcile.Result{}, nil
	}

	config, err := r.getRestConfigFromInstance(ctx, instance)
	if err != nil {
		rlog.Error(err, "unable to get restconfig for", "instance", instance)
		return r.ManageError(ctx, instance, err)
	}

	lockedPatches, err := lockedpatch.GetLockedPatches(instance.Spec.Patches, config, rlog)

	if err != nil {
		rlog.Error(err, "unable to get patches for", "instance", instance)
		return r.ManageError(ctx, instance, err)
	}

	err = r.UpdateLockedResourcesWithRestConfig(ctx, instance, nil, lockedPatches, config)
	if err != nil {
		rlog.Error(err, "unable to update locked resources")
		return r.ManageError(ctx, instance, err)
	}

	return r.ManageSuccess(ctx, instance)

}

// SetupWithManager sets up the controller with the Manager.
func (r *PatchReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redhatcopv1alpha1.Patch{}).
		Watches(&source.Channel{Source: r.GetStatusChangeChannel()}, &handler.EnqueueRequestForObject{}).
		Complete(r)
}

func (r *PatchReconciler) getRestConfigFromInstance(ctx context.Context, instance *redhatcopv1alpha1.Patch) (*rest.Config, error) {
	rlog := log.FromContext(ctx)
	sa := corev1.ServiceAccount{}

	secretList := &corev1.SecretList{}
	err := r.GetClient().List(ctx, secretList, &client.ListOptions{
		Namespace: instance.GetNamespace(),
	})

	var saSecret corev1.Secret

	if err != nil {
		rlog.Error(err, "unable to retrieve secrets", "in namespace", instance.GetNamespace())
		return nil, err
	}
	for _, secret := range secretList.Items {
		if saname, ok := secret.Annotations["kubernetes.io/service-account.name"]; ok {
			if secret.Type == corev1.SecretTypeServiceAccountToken && saname == instance.Spec.ServiceAccountRef.Name {
				if _, ok := secret.Data["token"]; ok {
					saSecret = secret
					break
				} else {
					return nil, errs.New("unable to find \"token\" key in secret" + instance.GetNamespace() + "/" + secret.Name)
				}
			}
		}
	}
	// if the map is still empty we test the old approach, pre kube 1.21
	if _, ok := saSecret.Data["token"]; !ok {
		err := r.GetClient().Get(ctx, types.NamespacedName{Name: instance.Spec.ServiceAccountRef.Name, Namespace: instance.GetNamespace()}, &sa)
		if err != nil {
			rlog.Error(err, "unable to get the specified", "service account", types.NamespacedName{Name: instance.Spec.ServiceAccountRef.Name, Namespace: instance.GetNamespace()})
			return &rest.Config{}, err
		}
		var tokenSecret corev1.Secret
		for _, secretRef := range sa.Secrets {
			secret := corev1.Secret{}
			err := r.GetClient().Get(ctx, types.NamespacedName{Name: secretRef.Name, Namespace: instance.GetNamespace()}, &secret)
			if err != nil {
				rlog.Error(err, "(ignoring) unable to get ", "ref secret", types.NamespacedName{Name: secretRef.Name, Namespace: instance.GetNamespace()})
				continue
			}
			if secret.Type == "kubernetes.io/service-account-token" {
				tokenSecret = secret
				break
			}
		}
		if tokenSecret.Data == nil {
			err = errs.New("unable to find secret of type kubernetes.io/service-account-token")
			rlog.Error(err, "unable to find secret of type kubernetes.io/service-account-token for", "service account", sa)
			return &rest.Config{}, err
		}
		if _, ok := tokenSecret.Data["token"]; ok {
			saSecret = tokenSecret
		} else {
			return nil, errs.New("unable to find \"token\" key in secret" + instance.GetNamespace() + "/" + tokenSecret.Name)
		}
	}
	// if we got here the map should be filled up
	config := rest.Config{
		Host:        r.GetRestConfig().Host,
		BearerToken: string(saSecret.Data["token"]),
		TLSClientConfig: rest.TLSClientConfig{
			CAData: saSecret.Data["ca.crt"],
		},
	}
	return &config, nil
}

// manageCleanupLogic delete resources. We don't touch pacthes because we cannot undo them.
func (r *PatchReconciler) manageCleanUpLogic(ctx context.Context, instance *redhatcopv1alpha1.Patch) error {
	rlog := log.FromContext(ctx)
	err := r.Terminate(instance, true)
	if err != nil {
		rlog.Error(err, "unable to terminate enforcing reconciler for", "instance", instance)
		return err
	}
	return nil
}

//ManageError manage error sets an error status in the CR and fires an event, finally it returns the error so the operator can re-attempt
func (er *PatchReconciler) ManageError(ctx context.Context, instance *redhatcopv1alpha1.Patch, issue error) (reconcile.Result, error) {
	rlog := log.FromContext(ctx)
	er.GetRecorder().Event(instance, "Warning", "ProcessingError", issue.Error())
	condition := metav1.Condition{
		Type:               apis.ReconcileError,
		LastTransitionTime: metav1.Now(),
		Message:            issue.Error(),
		ObservedGeneration: instance.GetGeneration(),
		Reason:             apis.ReconcileErrorReason,
		Status:             metav1.ConditionTrue,
	}
	instance.Status.Conditions = apis.AddOrReplaceCondition(condition, instance.Status.Conditions)
	instance.Status.PatchStatuses = er.GetLockedPatchStatuses(instance)
	err := er.GetClient().Status().Update(ctx, instance)
	if err != nil {
		if errors.IsResourceExpired(err) {
			rlog.Info("unable to update status for", "object version", instance.GetResourceVersion(), "resource version expired, will trigger another reconcile cycle", "")
		} else {
			rlog.Error(err, "unable to update status for", "object", instance)
		}
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, issue
}

// ManageSuccess will update the status of the CR and return a successful reconcile result
func (er *PatchReconciler) ManageSuccess(ctx context.Context, instance *redhatcopv1alpha1.Patch) (reconcile.Result, error) {
	rlog := log.FromContext(ctx)

	condition := metav1.Condition{
		Type:               apis.ReconcileSuccess,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: instance.GetGeneration(),
		Reason:             apis.ReconcileSuccessReason,
		Status:             metav1.ConditionTrue,
	}
	instance.Status.Conditions = apis.AddOrReplaceCondition(condition, instance.Status.Conditions)
	//we expect only one element
	instance.Status.PatchStatuses = er.GetLockedPatchStatuses(instance)
	err := er.GetClient().Status().Update(ctx, instance)
	if err != nil {
		if errors.IsResourceExpired(err) {
			rlog.Info("unable to update status for", "object version", instance.GetResourceVersion(), "resource version expired, will trigger another reconcile cycle", "")
		} else {
			rlog.Error(err, "unable to update status for", "object", instance)
		}
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}
