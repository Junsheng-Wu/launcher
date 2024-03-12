package utils

import (
	"context"
	ecnsv1 "easystack.com/plan/api/v1"
	"easystack.com/plan/pkg/scope"
	"fmt"
	clusteropenstack "github.com/easystack/cluster-api-provider-openstack/api/v1alpha6"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clusterapi "sigs.k8s.io/cluster-api/api/v1beta1"
	machineerror "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"
)

const (
	retryWaitInstanceStatus = 10 * time.Second
	timeoutInstanceReady    = 120 * time.Second
)

// MachineToInfrastructureMapFunc returns a handler.ToRequestsFunc that watches for
// Machine events and returns reconciliation requests for an infrastructure provider object.
func MachineToInfrastructureMapFunc(ctx context.Context, c client.Client) handler.MapFunc {
	return func(o client.Object) []reconcile.Request {
		machine, ok := o.(*clusterapi.Machine)
		if !ok {
			return nil
		}
		// Return early if the InfrastructureRef is nil.
		if machine.ObjectMeta.Labels[ecnsv1.MachineSetClusterLabelName] == "" {
			return nil
		}
		clusterName := machine.ObjectMeta.Labels[ecnsv1.MachineSetClusterLabelName]
		var cluster clusterapi.Cluster
		err := c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: machine.Namespace}, &cluster)
		if err != nil {
			return nil
		}
		planName := cluster.ObjectMeta.Labels[LabelEasyStackPlan]
		if planName == "" {
			return nil
		}
		return []reconcile.Request{
			{
				NamespacedName: client.ObjectKey{
					Namespace: cluster.Namespace,
					Name:      planName,
				},
			},
		}
	}
}

// GetOwnerMachine returns the Machine object owning the current resource.
func GetOwnerMachine(ctx context.Context, c client.Client, obj metav1.ObjectMeta) (*clusterapi.Machine, error) {
	for _, ref := range obj.OwnerReferences {
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return nil, err
		}
		if ref.Kind == "Machine" && gv.Group == clusterapi.GroupVersion.Group {
			return GetMachineByName(ctx, c, obj.Namespace, ref.Name)
		}
	}
	return nil, nil
}

// GetMachineByName finds and return a Machine object using the specified params.
func GetMachineByName(ctx context.Context, c client.Client, namespace, name string) (*clusterapi.Machine, error) {
	m := &clusterapi.Machine{}
	key := client.ObjectKey{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, m); err != nil {
		return nil, err
	}
	return m, nil
}

// WaitAllMachineReady wait all machine ready
// 1. poll wait machine ready
// 2. if machine is not ready status and FailureReason!=nil,we need give error about instance id and error message
// 3.we need update plan status and give information once if not all ready in two minutes.
func WaitAllMachineReady(ctx context.Context, scope *scope.Scope, cli client.Client, plan *ecnsv1.Plan) error {

	//TODO check new machine has created and InfrastructureRef !=nil,or give a reason to user
	var errorMessage = make(map[string]ecnsv1.MachineFailureReason)

	err := PollImmediate(retryWaitInstanceStatus, timeoutInstanceReady, func() (bool, error) {
		var openstackMachines clusteropenstack.OpenStackMachineList
		labels := map[string]string{ecnsv1.MachineSetClusterLabelName: plan.Spec.ClusterName}
		err := cli.List(ctx, &openstackMachines, client.InNamespace(plan.Namespace), client.MatchingLabels(labels))
		if err != nil {
			return false, err
		}
		var NoReadyCount int64
		for _, oMachine := range openstackMachines.Items {
			if !oMachine.Status.Ready {
				// get condition error,such as instance quotas exceed
				conditions := oMachine.GetConditions()
				condition := judgeConditions(conditions)
				if condition.Reason != "" {
					conditionError := machineerror.MachineStatusError(condition.Reason)
					InstanceError := ecnsv1.MachineFailureReason{
						Type:    ecnsv1.ConditionError,
						Reason:  &conditionError,
						Message: &condition.Message,
					}
					errorMessage[oMachine.Name] = InstanceError
				}
				// get instance error message
				if oMachine.Status.FailureReason != nil || oMachine.Status.FailureMessage != nil {
					InstanceError := ecnsv1.MachineFailureReason{
						Type:    ecnsv1.InstanceError,
						Reason:  oMachine.Status.FailureReason,
						Message: oMachine.Status.FailureMessage,
					}
					errorMessage[oMachine.Name] = InstanceError
				}

				NoReadyCount++
			}
		}
		if NoReadyCount > 0 && len(errorMessage) == 0 {
			return false, nil
		} else if NoReadyCount > 0 && len(errorMessage) > 0 {
			return true, errors.New("VM create failed")
		}
		return true, nil
	})

	if len(errorMessage) > 0 {
		plan = ecnsv1.SetPlanPhase(plan, ecnsv1.VM, ecnsv1.Failed)
		plan.Status.VMFailureReason = errorMessage
		//try to update plan status
		if errM := cli.SubResource("status").Update(ctx, plan); errM != nil {
			scope.Logger.Error(errM, "update plan vm status failed")
		}
		return errors.Wrapf(err, fmt.Sprintf("wait for OpenstackMachine ready error,cluster:%s", plan.Spec.ClusterName))
	}
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("wait for OpenstackMachine ready timeout,cluster:%s", plan.Spec.ClusterName))
	}
	scope.Logger.Info("machine all has ready,continue task")
	return nil
}

// judgeConditions judge the conditions of the machine when the condition.severity is error,return this condition
func judgeConditions(conditions clusterapi.Conditions) clusterapi.Condition {
	for _, con := range conditions {
		if con.Severity == clusterapi.ConditionSeverityError {
			return con
		}
	}
	return clusterapi.Condition{}

}
