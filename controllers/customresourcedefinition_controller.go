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
	"sync"

	"github.com/redhat-cop/operator-utils/pkg/util/discoveryclient"
	apiextension "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	openapi "k8s.io/kube-openapi/pkg/util/proto"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func NewCustomResourceDefinitionReconciler(restConfig *rest.Config) *CustomResourceDefinitionReconciler {
	customResourceDefinitionReconciler := CustomResourceDefinitionReconciler{
		restConfig: restConfig,
		modelLock:  sync.Mutex{},
	}
	ctx := context.TODO()
	ctx = log.IntoContext(ctx, ctrl.Log.WithName("NewCustomResourceDefinitionReconciler"))
	err := customResourceDefinitionReconciler.loadModels(ctx)
	if err != nil {
		panic(err)
	}
	return &customResourceDefinitionReconciler
}

// CustomResourceDefinitionReconciler reconciles a CustomResourceDefinition object
type CustomResourceDefinitionReconciler struct {
	restConfig    *rest.Config
	modelLock     sync.Mutex
	openapiModels openapi.Models
}

func (r *CustomResourceDefinitionReconciler) GetModels() openapi.Models {
	r.modelLock.Lock()
	defer r.modelLock.Unlock()
	return r.openapiModels
}

func (r *CustomResourceDefinitionReconciler) setModels(openapiModels openapi.Models) {
	r.modelLock.Lock()
	defer r.modelLock.Unlock()
	r.openapiModels = openapiModels
}

//+kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=users;groups;serviceaccounts,verbs=impersonate

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CustomResourceDefinition object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *CustomResourceDefinitionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	rlog := log.FromContext(ctx)
	err := r.loadModels(ctx)
	if err != nil {
		rlog.Error(err, "unable to load models")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CustomResourceDefinitionReconciler) loadModels(ctx context.Context) error {
	rlog := log.FromContext(ctx)
	ctx = context.WithValue(ctx, "restConfig", r.restConfig)

	client, err := discoveryclient.GetDiscoveryClient(ctx)
	if err != nil {
		rlog.Error(err, "unable to get discovery client")
		return err
	}
	openapidoc, err := client.OpenAPISchema()

	if err != nil {
		rlog.Error(err, "unable to openapi schema")
		return err
	}
	openapiModels, err := openapi.NewOpenAPIData(openapidoc)
	if err != nil {
		rlog.Error(err, "unable to parse", "openapidoc", openapidoc)
		return err
	}

	r.setModels(openapiModels)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CustomResourceDefinitionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("openapi-watcher").
		For(&apiextension.CustomResourceDefinition{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(object client.Object) bool {
			return false
		}))).
		Watches(&source.Kind{Type: &apiextension.CustomResourceDefinition{}}, handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
			return []reconcile.Request{{
				NamespacedName: types.NamespacedName{
					Namespace: "alwaysthesame",
					Name:      "alwaysthesame",
				},
			}}
		})).
		Complete(r)
}
