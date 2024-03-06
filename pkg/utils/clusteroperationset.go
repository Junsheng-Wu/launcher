package utils

import (
	"fmt"
	"context"
	"encoding/json"

	ecnsv1 "easystack.com/plan/api/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/jsonmergepatch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PatchAnsiblePlan makes patch request to the MachineSet object.
func PatchClusterOperationSet(ctx context.Context, cli client.Client, cur, mod *ecnsv1.ClusterOperationSet) error {
	curJSON, err := json.Marshal(cur)
	if err != nil {
		return fmt.Errorf("failed to serialize current MachineSet object: %s", err)
	}

	modJSON, err := json.Marshal(mod)
	if err != nil {
		return fmt.Errorf("failed to serialize modified MachineSet object: %s", err)
	}
	patch, err := jsonmergepatch.CreateThreeWayJSONMergePatch(curJSON, modJSON, curJSON)
	if err != nil {
		return fmt.Errorf("failed to create 2-way merge patch: %s", err)
	}
	if len(patch) == 0 || string(patch) == "{}" {
		return nil
	}
	patchObj := client.RawPatch(types.MergePatchType, patch)
	fmt.Println(string(patch))
	// client patch ansible plan object
	err = cli.Patch(ctx, cur, patchObj)
	if err != nil {
		return fmt.Errorf("failed to patch ClusterOperationSet object %s/%s: %s", cur.Namespace, cur.Name, err)
	}
	// update status after all job has done
	curStatusJSON, err := json.Marshal(cur.Status)
	if err != nil {
		return fmt.Errorf("failed to serialize current MachineSet object: %s", err)
	}
	modStatusJSON, err := json.Marshal(mod.Status)
	if err != nil {
		return fmt.Errorf("failed to serialize modified MachineSet object: %s", err)
	}
	patchStatus, err := jsonmergepatch.CreateThreeWayJSONMergePatch(curStatusJSON, modStatusJSON, curStatusJSON)
	if err != nil {
		return fmt.Errorf("failed to create 2-way merge patch status value: %s", err)
	}
	if len(patchStatus) == 0 || string(patchStatus) == "{}" {
		return nil
	}
	patchStatus, err = CreateStatusThreeWayJSONMergePatch(patchStatus)
	if err != nil {
		return fmt.Errorf("failed to create 2-way merge patch status: %s", err)
	}
	patchStatusObj := client.RawPatch(types.MergePatchType, patchStatus)
	fmt.Println(string(patchStatus))
	err = cli.SubResource("status").Patch(ctx, cur, patchStatusObj)
	if err != nil {
		return fmt.Errorf("failed to patch ClusterOperationSet object status %s/%s: %s %s", cur.Namespace, cur.Name, err, string(patchStatus))
	}
	return nil
}
