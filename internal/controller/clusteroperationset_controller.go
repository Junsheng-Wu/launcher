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
	"github.com/heimdalr/dag"

	ecnsv1 "easystack.com/plan/api/v1"
	"easystack.com/plan/pkg/utils"
	"github.com/go-logr/logr"
	clusteroperationv1alpha1 "github.com/kubean-io/kubean-api/apis/clusteroperation/v1alpha1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ClusterOperationSetReconciler reconciles a ClusterOperationSet object
type ClusterOperationSetReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	EventRecorder record.EventRecorder
}

//+kubebuilder:rbac:groups=easystack.com,resources=clusteroperationsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=easystack.com,resources=clusteroperationsets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=easystack.com,resources=clusteroperationsets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ClusterOperationSet object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *ClusterOperationSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	var (
		log = log.FromContext(ctx)
	)
	// Fetch the OpenStackMachine instance.
	operationSet := &ecnsv1.ClusterOperationSet{}
	err := r.Client.Get(ctx, req.NamespacedName, operationSet)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	log = log.WithValues("ClusterOperationSet", operationSet.Name)

	operationSetBak := operationSet.DeepCopy()

	patchHelper, err := patch.NewHelper(operationSet, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	if operationSet.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !StringInArray(ecnsv1.AnsibleFinalizer, operationSet.ObjectMeta.Finalizers) {
			operationSet.ObjectMeta.Finalizers = append(operationSet.ObjectMeta.Finalizers, ecnsv1.AnsibleFinalizer)
			if err := r.Update(context.Background(), operationSet); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		if StringInArray(ecnsv1.ClusterOperationSetFinalizer, operationSet.ObjectMeta.Finalizers) {
			log.Info("delete ClusterOperationSet CR", "Namespace", operationSet.ObjectMeta.Namespace, "Name", operationSet.Name)
			// remove our finalizer from the list and update it.
			var found bool
			operationSet.ObjectMeta.Finalizers, found = RemoveString(ecnsv1.ClusterOperationSetFinalizer, operationSet.ObjectMeta.Finalizers)
			if found {
				if err := patchHelper.Patch(ctx, operationSet); err != nil {
					return ctrl.Result{}, err
				}
			}
			r.EventRecorder.Eventf(operationSet, corev1.EventTypeNormal, ClusterOperationSetDeleteEvent, "Delete ClusterOperationSet")

			return r.reconcileDelete(ctx, operationSet)
		}
	}

	defer func() {
		if operationSet.ObjectMeta.DeletionTimestamp.IsZero() {
			r.EventRecorder.Eventf(operationSet, corev1.EventTypeNormal, ClusterOperationSetUpdateEvent, "patch %s/%s ClusterOperationSet status", operationSet.Namespace, operationSet.Name)
			if err = utils.PatchClusterOperationSet(ctx, r.Client, operationSetBak, operationSet); err != nil {
				if reterr == nil {
					reterr = errors.Wrapf(err, "error patching ClusterOperationSet status %s/%s", operationSet.Namespace, operationSet.Name)
				}
			}
		}
	}()

	return r.reconcileNormal(ctx, log, patchHelper, operationSet)
}

func (r *ClusterOperationSetReconciler) reconcileNormal(ctx context.Context, log logr.Logger, patchHelper *patch.Helper, cos *ecnsv1.ClusterOperationSet) (ctrl.Result, error) {
	log.Info("Reconciling ClusterOperationSet resource")
	err := r.SyncClusterOperations(ctx, cos)
	if err != nil {
		r.EventRecorder.Eventf(cos, corev1.EventTypeNormal, ClusterOperationSetUpdateEvent, "Create %s/%s ClusterOperationSet status", cos.Namespace, cos.Name)
		return ctrl.Result{}, err
	}

	var (
		newclusterOperationSet    = ecnsv1.ClusterOperationSet{}
		clusterOperationSetStatus = ecnsv1.ClusterOperationSetStatus{}
		clusterOperation          = clusteroperationv1alpha1.ClusterOperation{}
	)
	newclusterOperationSet = *cos.DeepCopy()

	for i, op := range cos.Spec.ClusterOperations {
		var status = ecnsv1.ClusterOperationStatus{}
		err := r.Client.Get(ctx, types.NamespacedName{Namespace: op.Namespace, Name: op.Name}, &clusterOperation)
		if err != nil {
			if apierrors.IsNotFound(err) {
				status = ecnsv1.ClusterOperationStatus{
					OperationName: "",
					Status:        "",
					Action:        "",
				}
			} else {
				return ctrl.Result{}, err
			}

		} else {
			status = ecnsv1.ClusterOperationStatus{
				OperationName: clusterOperation.Name,
				Status:        clusterOperation.Status.Status,
				Action:        clusterOperation.Spec.Action,
			}
		}
		newclusterOperationSet.Spec.ClusterOperations[i].Status = status.Status

		clusterOperationSetStatus.ClusterOperationStatusList = append(clusterOperationSetStatus.ClusterOperationStatusList, status)
	}

	newclusterOperationSet.Status = clusterOperationSetStatus

	err = patchHelper.Patch(ctx, &newclusterOperationSet)
	if err != nil {
		log.Error(err, "Update clusterOperation failed!")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ClusterOperationSetReconciler) reconcileDelete(ctx context.Context, cos *ecnsv1.ClusterOperationSet) (ctrl.Result, error) {
	clusterOperations := cos.Spec.ClusterOperations
	for _, co := range clusterOperations {
		clusterOperation := &clusteroperationv1alpha1.ClusterOperation{}
		err := r.Client.Get(ctx, types.NamespacedName{Name: co.Name, Namespace: co.Namespace}, clusterOperation)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Log.Info("ClusterOperation %s has already been deleted", co.Name)
				return ctrl.Result{}, nil
			}
		}

		err = r.Client.Delete(ctx, clusterOperation)
		if err != nil {
			r.EventRecorder.Eventf(cos, corev1.EventTypeNormal, ClusterOperationsDeleteEvent, "Delete ClusterOperations %s failed: %s", clusterOperation.Name, err.Error())
			return ctrl.Result{}, err
		}
		r.EventRecorder.Eventf(cos, corev1.EventTypeNormal, ClusterOperationsDeleteEvent, "Start delete ClusterOperations: %s", clusterOperation.Name)
	}
	return ctrl.Result{}, nil
}

func (r *ClusterOperationSetReconciler) SyncClusterOperations(ctx context.Context, cos *ecnsv1.ClusterOperationSet) error {
	var (
		clusterOperations       = cos.Spec.ClusterOperations
		mapClusterOperationSide = map[string]*utils.OperationSide{}
	)
	d := dag.NewDAG()

	for _, co := range clusterOperations {
		side := utils.NewOperationSide(co)
		mapClusterOperationSide[side.Id] = side
	}

	visitor := utils.NewOperationVisitor(d, mapClusterOperationSide, cos.Spec.SideMap)
	d.OrderedWalk(visitor)

	// execute root
	executableSides := visitor.GetExecutableSides()
	if len(executableSides) > 0 {
		for _, es := range executableSides {
			oldCps := &clusteroperationv1alpha1.ClusterOperation{}
			err := r.Client.Get(ctx, types.NamespacedName{Namespace: es.ClusterOps.Namespace, Name: es.ClusterOps.Name}, oldCps)
			if err != nil {
				if apierrors.IsNotFound(err) {
					es.ClusterOps.Status = utils.RunningStatus
					ops := CreateClusterOperationSet(es.ClusterOps)
					err = r.Client.Create(ctx, &ops)
					if err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterOperationSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ecnsv1.ClusterOperationSet{}).
		Complete(r)
}

func CreateClusterOperationSet(clusterOps *ecnsv1.ClusterOps) clusteroperationv1alpha1.ClusterOperation {
	var clusterOperation = clusteroperationv1alpha1.ClusterOperation{}
	clusterOperation.Name = clusterOps.Name
	clusterOperation.Status.Status = clusterOps.Status
	clusterOperation.Namespace = clusterOps.Namespace
	clusterOperation.Spec.Cluster = clusterOps.Cluster
	clusterOperation.Spec.Image = clusterOps.Image
	clusterOperation.Spec.Action = clusterOps.Action
	clusterOperation.Spec.ActionSource = (*clusteroperationv1alpha1.ActionSource)(&clusterOps.ActionSource)
	clusterOperation.Spec.ActionType = clusteroperationv1alpha1.ActionType(clusterOps.ActionType)
	clusterOperation.Spec.PreHook = clusterOps.PreHook
	return clusterOperation
}
