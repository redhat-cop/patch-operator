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
	"os"
	"time"

	"github.com/redhat-cop/operator-utils/pkg/util"
	"github.com/redhat-cop/operator-utils/pkg/util/apis"
	"github.com/redhat-cop/operator-utils/pkg/util/lockedresourcecontroller"
	"github.com/redhat-cop/operator-utils/pkg/util/lockedresourcecontroller/lockedpatch"
	redhatcopv1alpha1 "github.com/redhat-cop/patch-operator/api/v1alpha1"
	authv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
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
//+kubebuilder:rbac:groups="",resources=serviceaccounts/token,verbs=create

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

func getJWTToken(context context.Context, serviceAccountName string, kubeNamespace string) (string, error) {
	log := log.FromContext(context)

	restConfig := context.Value("restConfig").(*rest.Config)
	lenght, found := os.LookupEnv("SERVICE_ACCOUNT_TOKEN_EXPIRATION_DURATION")
	//default is 1 year
	defaultDuration, _ := time.ParseDuration("8760h")
	var duration time.Duration
	if found {
		parsedDuration, err := time.ParseDuration(lenght)
		if err != nil {
			log.Error(err, "unable to parse SERVICE_ACCOUNT_TOKEN_EXPIRATION_DURATION to duration, continuing with", "default duration", defaultDuration)
			duration = defaultDuration
		} else {
			duration = parsedDuration
		}
	} else {
		duration = defaultDuration
	}

	// we request a token valid for 1 year. This token will be refreshed when the pod restarts, or when the patch changes. We assume both of these events will happen with a frequency of more than once eveny year
	seconds := int64(duration.Seconds())
	treq := &authv1.TokenRequest{
		Spec: authv1.TokenRequestSpec{
			ExpirationSeconds: &seconds,
		},
	}

	clientset, err := kubernetes.NewForConfig(restConfig)

	if err != nil {
		log.Error(err, "unable to create kubernetes clientset")
		return "", err
	}

	treq, err = clientset.CoreV1().ServiceAccounts(kubeNamespace).CreateToken(context, serviceAccountName, treq, metav1.CreateOptions{})
	if err != nil {
		log.Error(err, "unable to create service account token request", "in namespace", kubeNamespace, "for service account", serviceAccountName)
		return "", err
	}

	log.Info("token expiration: " + treq.Status.ExpirationTimestamp.String())

	return treq.Status.Token, nil
}

func (r *PatchReconciler) getRestConfigFromInstance(ctx context.Context, instance *redhatcopv1alpha1.Patch) (*rest.Config, error) {
	rlog := log.FromContext(ctx)
	ctx = context.WithValue(ctx, "restConfig", r.GetRestConfig())
	token, err := getJWTToken(ctx, instance.Spec.ServiceAccountRef.Name, instance.GetNamespace())
	if err != nil {
		rlog.Error(err, "unable to retrieve token for", "service account", instance.Spec.ServiceAccountRef.Name, "in namespace", instance.GetNamespace())
		return nil, err
	}

	config := rest.Config{
		Host:        r.GetRestConfig().Host,
		BearerToken: token,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: r.GetRestConfig().CAData,
			CAFile: r.GetRestConfig().CAFile,
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
