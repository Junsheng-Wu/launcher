/*
Copyright 2023.

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

package controller

import (
	"context"
	"reflect"

	easystackcomv1 "easystack.com/plan/api/v1"
	ecnsv1 "easystack.com/plan/api/v1"
	"easystack.com/plan/pkg/utils"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// AnsiblePlanReconciler reconciles a AnsiblePlan object
type AnsiblePlanReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	EventRecorder record.EventRecorder
}

//+kubebuilder:rbac:groups=easystack.com,resources=ansibleplans,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=easystack.com,resources=ansibleplans/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=easystack.com,resources=ansibleplans/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AnsiblePlan object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *AnsiblePlanReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := log.FromContext(ctx)
	log.Info("start ansible reconcile")
	// Fetch the AnsiblePlan instance
	ansible := &easystackcomv1.AnsiblePlan{}
	err := r.Get(ctx, req.NamespacedName, ansible)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}
	var ansibleBak = ansible.DeepCopy()
	if !ansible.Spec.AutoRun {
		log.Info("ansible plan auto run is false,skip reconcile")
		return ctrl.Result{}, nil
	}
	patchHelper, err := patch.NewHelper(ansible, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	if ansible.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !StringInArray(ecnsv1.AnsibleFinalizer, ansible.ObjectMeta.Finalizers) {
			ansible.ObjectMeta.Finalizers = append(ansible.ObjectMeta.Finalizers, ecnsv1.AnsibleFinalizer)
			if err := r.Update(context.Background(), ansible); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		if StringInArray(ecnsv1.AnsibleFinalizer, ansible.ObjectMeta.Finalizers) {
			log.Info("delete ansible CR", "Namespace", ansible.ObjectMeta.Namespace, "Name", ansible.Name)
			// remove our finalizer from the list and update it.
			var found bool
			ansible.ObjectMeta.Finalizers, found = RemoveString(ecnsv1.AnsibleFinalizer, ansible.ObjectMeta.Finalizers)
			if found {
				if err := patchHelper.Patch(ctx, ansible); err != nil {
					return ctrl.Result{}, err
				}
			}
			r.EventRecorder.Eventf(ansible, corev1.EventTypeNormal, AnsiblePlanDeleteEvent, "Delete ansible plan")

			return r.reconcileDelete(ctx, log, patchHelper, ansible)
		}

	}

	// Handle non-deleted status and fields update
	defer func() {
		if ansible.ObjectMeta.DeletionTimestamp.IsZero() {
			r.EventRecorder.Eventf(ansible, corev1.EventTypeNormal, AnsiblePlanStatusUpdateEvent, "patch %s/%s ansible plan status", ansible.Namespace, ansible.Name)
			if err = utils.PatchAnsiblePlan(ctx, r.Client, ansibleBak, ansible); err != nil {
				if reterr == nil {
					reterr = errors.Wrapf(err, "error patching ansible plan status %s/%s", ansible.Namespace, ansible.Name)
				}
			}
		}
	}()

	r.EventRecorder.Eventf(ansible, corev1.EventTypeNormal, AnsiblePlanStartEvent, "Start ansible plan.")
	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, log, patchHelper, ansible)

}

func (r *AnsiblePlanReconciler) reconcileNormal(ctx context.Context, log logr.Logger, patchHelper *patch.Helper, ansible *easystackcomv1.AnsiblePlan) (ctrl.Result, error) {
	log.Info("Reconciling ansible plan resource")

	if ansible.Status.Done {
		log.Info("ansible plan is done, skip reconcile")
		return ctrl.Result{}, nil
	} else if ansible.Spec.MaxRetryTime <= ansible.Status.CurrentRetryTime {
		log.Info("The number of ansible plan retry times has reached the max retry times, skip reconcile.")
		return ctrl.Result{}, nil
	}

	err := utils.GetOrCreateSSHkeyFile(ctx, r.Client, ansible)
	if err != nil {
		r.EventRecorder.Eventf(ansible, corev1.EventTypeWarning, AnsiblePlanCreatedEvent, "Get ssh key failed: %s", err.Error())
		return ctrl.Result{}, err
	}
	r.EventRecorder.Eventf(ansible, corev1.EventTypeNormal, AnsiblePlanCreatedEvent, "Get ssh key success")

	err = utils.GetOrCreateInventoryFile(ctx, r.Client, ansible)
	if err != nil {
		r.EventRecorder.Eventf(ansible, corev1.EventTypeNormal, AnsiblePlanCreatedEvent, "Create inventory file failed: %s", err.Error())
		return ctrl.Result{}, err
	}
	r.EventRecorder.Eventf(ansible, corev1.EventTypeNormal, AnsiblePlanCreatedEvent, "Create inventory file success")

	err = utils.GetOrCreateVarsFile(ctx, r.Client, ansible)
	if err != nil {
		r.EventRecorder.Eventf(ansible, corev1.EventTypeNormal, AnsiblePlanCreatedEvent, "Create vars file failed: %s", err.Error())
		return ctrl.Result{}, err
	}
	r.EventRecorder.Eventf(ansible, corev1.EventTypeNormal, AnsiblePlanCreatedEvent, "Create vars file success")

	//TODO start ansible plan process,write pid log to file
	rt := ansible.Spec.MaxRetryTime - ansible.Status.CurrentRetryTime
	err = utils.Retry(rt, utils.AnsiblePlanExecuteInterval, func() error {
		ansible.Status.CurrentRetryTime += 1
		err = r.Client.Status().Update(ctx, ansible)
		if err != nil {
			return err
		}
		return utils.StartAnsiblePlan(ctx, r.Client, ansible)
	})
	if err != nil {
		r.EventRecorder.Eventf(ansible, corev1.EventTypeWarning, AnsiblePlanStartEvent, "Ansible plan execute failed: %s", err.Error())
		ansible.Status.Done = false
		return ctrl.Result{}, err
	}
	r.EventRecorder.Eventf(ansible, corev1.EventTypeWarning, AnsiblePlanStartEvent, "Ansible plan execute success")
	ansible.Status.Done = true

	return ctrl.Result{}, nil
}

func (r *AnsiblePlanReconciler) reconcileDelete(ctx context.Context, log logr.Logger, patchHelper *patch.Helper, ansible *easystackcomv1.AnsiblePlan) (ctrl.Result, error) {
	//Nothing to do
	return ctrl.Result{}, nil
}

func deleteAnsibleSSHKeySecret(ctx context.Context, client client.Client, ansible *ecnsv1.AnsiblePlan) error {
	secretName := ansible.Spec.SSHSecret
	//get secret by name secretName
	secret := &corev1.Secret{}
	err := client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: ansible.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Log.Info("SSHKeySecert has already been deleted")
			return nil
		}
	}

	err = client.Delete(ctx, secret)
	if err != nil {
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AnsiblePlanReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(
			&easystackcomv1.AnsiblePlan{},
			builder.WithPredicates(
				predicate.Funcs{
					// Avoid reconciling if the event triggering the reconciliation is related to incremental status updates
					UpdateFunc: func(e event.UpdateEvent) bool {
						oldCluster := e.ObjectOld.(*easystackcomv1.AnsiblePlan).DeepCopy()
						newCluster := e.ObjectNew.(*easystackcomv1.AnsiblePlan).DeepCopy()
						oldCluster.Status = easystackcomv1.AnsiblePlanStatus{}
						newCluster.Status = easystackcomv1.AnsiblePlanStatus{}
						oldCluster.ObjectMeta.ResourceVersion = ""
						newCluster.ObjectMeta.ResourceVersion = ""
						return !reflect.DeepEqual(oldCluster, newCluster)
					},
				},
			),
		).
		Complete(r)
}
