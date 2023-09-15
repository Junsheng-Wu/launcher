package utils

import (
	"context"
	ecnsv1 "easystack.com/plan/api/v1"
	"easystack.com/plan/pkg/scope"
	"encoding/json"
	"fmt"
	clusteropenstack "github.com/easystack/cluster-api-provider-openstack/api/v1alpha6"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/jsonmergepatch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

// DiffReporter is a simple custom reporter that only records differences
// detected during comparison.
type DiffReporter struct {
	path            cmp.Path
	diffs           []string
	UpScale         bool
	DownScale       bool
	AdditionalNodes []*ecnsv1.AnsibleNode
	NodesUpdate     bool
}

func (r *DiffReporter) PushStep(ps cmp.PathStep) {
	r.path = append(r.path, ps)
}

func (r *DiffReporter) Report(rs cmp.Result) {
	if !rs.Equal() {
		vx, vy := r.path.Last().Values()
		r.diffs = append(r.diffs, fmt.Sprintf("%#v:\n\t-: %+v\n\t+: %+v\n", r.path, vx, vy))
		if vx.IsValid() && vx.Type().String() == "*v1.AnsibleNode" {
			r.DownScale = true
			r.AdditionalNodes = append(r.AdditionalNodes, vx.Interface().(*ecnsv1.AnsibleNode))
		} else if vy.IsValid() && vy.Type().String() == "*v1.AnsibleNode" {
			r.UpScale = true
			r.AdditionalNodes = append(r.AdditionalNodes, vy.Interface().(*ecnsv1.AnsibleNode))
		} else {
			r.NodesUpdate = true
		}

	}
}

func (r *DiffReporter) PopStep() {
	r.path = r.path[:len(r.path)-1]
}

func (r *DiffReporter) String() string {
	return strings.Join(r.diffs, "\n")
}

const (
	retryWaitInstanceStatus = 10 * time.Second
	timeoutInstanceReady    = 120 * time.Second
)

func WaitAnsiblePlan(ctx context.Context, scope *scope.Scope, cli client.Client, plan *ecnsv1.Plan) error {
	//TODO check new machine has created and InfrastructureRef !=nil,or give a reason to user
	err := PollImmediate(retryWaitInstanceStatus, timeoutInstanceReady, func() (bool, error) {
		var openstackMachines clusteropenstack.OpenStackMachineList
		labels := map[string]string{ecnsv1.MachineSetClusterLabelName: plan.Spec.ClusterName}
		err := cli.List(ctx, &openstackMachines, client.InNamespace(plan.Namespace), client.MatchingLabels(labels))
		if err != nil {
			return false, err
		}
		for _, oMachine := range openstackMachines.Items {
			if !oMachine.Status.Ready {
				return false, nil
			}
		}
		// get bastion information
		// get openstack cluster status
		var OCluster clusteropenstack.OpenStackCluster
		err = cli.Get(ctx, types.NamespacedName{Name: plan.Spec.ClusterName, Namespace: plan.Namespace}, &OCluster)
		if err != nil {
			return false, err
		}
		if OCluster.Status.Bastion == nil {
			return false, nil
		}
		if OCluster.Status.Bastion.State != clusteropenstack.InstanceStateActive {
			return false, nil
		}
		if OCluster.Status.Bastion.FloatingIP == "" {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("wait for OpenstackMachine ready timeout,cluster:%s", plan.Spec.ClusterName))
	}
	scope.Logger.Info("machine all has ready,continue task")
	return nil
}

func IsUpgradeNeeded(ansibleOld *ecnsv1.AnsiblePlan, ansibleNew *ecnsv1.AnsiblePlan) (bool, error) {
	// v1.18.3 -> v1.18.4 convert to 1.18.3 -> 1.18.
	// Version Control Reference https://semver.org/
	oldVersion := strings.TrimPrefix(ansibleOld.Spec.Version, "v")
	newVersion := strings.TrimPrefix(ansibleNew.Spec.Version, "v")
	v1, err := version.NewVersion(oldVersion)
	if err != nil {
		return false, err
	}
	v2, err := version.NewVersion(newVersion)
	if err != nil {
		return false, err
	}
	if v1.LessThan(v2) {
		return true, nil
	}
	// Maybe user want to downgrade,but we should support this?
	return false, nil

}

func CreateAnsiblePlan(ctx context.Context, scope *scope.Scope, cli client.Client, plan *ecnsv1.Plan) ecnsv1.AnsiblePlan {
	var ansiblePlan ecnsv1.AnsiblePlan
	ansiblePlan.Name = plan.Name
	ansiblePlan.Namespace = plan.Namespace
	ansiblePlan.Spec.Type = ecnsv1.ExecTypeInstall
	ansiblePlan.Spec.ClusterName = plan.Spec.ClusterName
	ansiblePlan.Spec.AutoRun = plan.Spec.AnsiblePlanAuto
	ansiblePlan.Spec.SupportPython3 = plan.Spec.SupportPython3
	ansiblePlan.Spec.MaxRetryTime = plan.Spec.MaxRetryTime
	ansiblePlan.Status.Done = false
	ansiblePlan.Status.CurrentRetryTime = 0
	secretName := fmt.Sprintf("%s%s", plan.Name, SSHSecretSuffix)
	ansiblePlan.Spec.SSHSecret = secretName
	ansiblePlan.Spec.Version = plan.Spec.K8sVersion
	var nodePools []*ecnsv1.AnsibleNode
	for _, machine := range plan.Status.OpenstackMachineList {
		nodePools = append(nodePools, &ecnsv1.AnsibleNode{
			Name:                     machine.Name,
			AnsibleHost:              machine.Status.Addresses[0].Address,
			AnsibleIP:                machine.Status.Addresses[0].Address,
			MemoryReserve:            -4,
			AnsibleSSHPrivateKeyFile: "",
		})
	}
	ansiblePlan.Spec.Install = &ecnsv1.AnsibleInstall{}
	ansiblePlan.Spec.Install.Bastion = &ecnsv1.AnsibleNode{}
	ansiblePlan.Spec.Install.Bastion.Name = "bastion"
	ansiblePlan.Spec.Install.Bastion.AnsibleHost = plan.Status.Bastion.FloatingIP
	ansiblePlan.Spec.Install.Bastion.AnsibleIP = plan.Status.Bastion.FloatingIP
	ansiblePlan.Spec.Install.NodePools = nodePools
	ansiblePlan.Spec.Install.OtherGroup = make(map[string][]string, 5)
	for _, machineSet := range plan.Status.InfraMachine {
		switch machineSet.Role {
		case ecnsv1.MasterSetRole:
			for name, _ := range machineSet.IPs {
				ansiblePlan.Spec.Install.KubeMaster = append(ansiblePlan.Spec.Install.KubeMaster, name)
			}
		case ecnsv1.WorkSetRole:
			for name, _ := range machineSet.IPs {
				ansiblePlan.Spec.Install.KubeNode = append(ansiblePlan.Spec.Install.KubeNode, name)
			}
		case ecnsv1.PrometheusSetRole:
			for name, _ := range machineSet.IPs {
				ansiblePlan.Spec.Install.KubePrometheus = append(ansiblePlan.Spec.Install.KubePrometheus, name)
				ansiblePlan.Spec.Install.KubeNode = append(ansiblePlan.Spec.Install.KubeNode, name)
			}
		case ecnsv1.LogSetRole:
			for name, _ := range machineSet.IPs {
				ansiblePlan.Spec.Install.KubeLog = append(ansiblePlan.Spec.Install.KubeLog, name)
				ansiblePlan.Spec.Install.KubeNode = append(ansiblePlan.Spec.Install.KubeNode, name)
			}
		case ecnsv1.IngressSetRole:
			for name, _ := range machineSet.IPs {
				ansiblePlan.Spec.Install.KubeIngress = append(ansiblePlan.Spec.Install.KubeIngress, name)
				ansiblePlan.Spec.Install.KubeNode = append(ansiblePlan.Spec.Install.KubeNode, name)
			}
		case ecnsv1.Etcd:
			for name, _ := range machineSet.IPs {
				ansiblePlan.Spec.Install.Etcd = append(ansiblePlan.Spec.Install.Etcd, name)
			}
		default:
			for name, _ := range machineSet.IPs {
				ansiblePlan.Spec.Install.OtherGroup[machineSet.Role] = append(ansiblePlan.Spec.Install.OtherGroup[machineSet.Role], name)
				ansiblePlan.Spec.Install.KubeNode = append(ansiblePlan.Spec.Install.KubeNode, name)
			}
		}
	}
	// when etcd is not set,use master as etcd
	if len(ansiblePlan.Spec.Install.Etcd) == 0 {
		ansiblePlan.Spec.Install.Etcd = ansiblePlan.Spec.Install.KubeMaster
	}
	ansiblePlan.Spec.Install.OtherAnsibleOpts = make(map[string]string, 40)
	// add test/vars param
	if plan.Spec.PodCidr != "" {
		ansiblePlan.Spec.Install.OtherAnsibleOpts["kube_pods_subnet"] = plan.Spec.PodCidr
	}
	if plan.Spec.SvcCidr != "" {
		ansiblePlan.Spec.Install.OtherAnsibleOpts["kube_service_addresses"] = plan.Spec.SvcCidr
	}

	// TODO need support more cni
	if plan.Spec.CniType != "" {
		// TODO ansiblePlan.Spec.Install.OtherAnsibleOpts["kube_network_plugin"] = plan.Spec.CniType
		ansiblePlan.Spec.Install.OtherAnsibleOpts["kube_network_plugin"] = "flannel"
	}
	if plan.Spec.CniWorkMode != "" {
		// TODO ansiblePlan.Spec.Install.OtherAnsibleOpts["flannel_backend_type"] = plan.Spec.CniWorkMode
		if plan.Spec.NetMode != ecnsv1.NetWorkExist {
			ansiblePlan.Spec.Install.OtherAnsibleOpts["flannel_backend_type"] = "host-gw"
		} else {
			ansiblePlan.Spec.Install.OtherAnsibleOpts["flannel_backend_type"] = "vxlan"
		}
	}
	if plan.Spec.Monitor.PvcType != "" {
		ansiblePlan.Spec.Install.OtherAnsibleOpts["local_storageclass"] = plan.Spec.Monitor.PvcType
	}
	if plan.Spec.Monitor.PvcCap != "" {
		ansiblePlan.Spec.Install.OtherAnsibleOpts["prometheus_pv_size"] = plan.Spec.Monitor.PvcCap
	}
	if plan.Spec.Monitor.AutoClearDays != "" {
		ansiblePlan.Spec.Install.OtherAnsibleOpts["prometheus_retention_time"] = plan.Spec.Monitor.AutoClearDays
	}
	for key, value := range plan.Spec.OtherAnsibleOpts {
		ansiblePlan.Spec.Install.OtherAnsibleOpts[key] = value
	}
	var curIngressHA, curMasterHA ecnsv1.InfraMachine
	if !plan.Spec.LBEnable {
		for index, HA := range plan.Status.InfraMachine {
			if HA.Role == ecnsv1.IngressSetRole {
				curIngressHA = plan.Status.InfraMachine[index]
			}
			if HA.Role == ecnsv1.MasterSetRole {
				curMasterHA = plan.Status.InfraMachine[index]
			}
		}
		_, IngresHAExisted := ansiblePlan.Spec.Install.OtherAnsibleOpts["ingress_virtual_vip"]
		if !IngresHAExisted {
			ansiblePlan.Spec.Install.OtherAnsibleOpts["ingress_virtual_vip"] = curIngressHA.HAPrivateIP
		}
		_, MasterHAExisted := ansiblePlan.Spec.Install.OtherAnsibleOpts["master_virtual_vip"]
		if !MasterHAExisted {
			ansiblePlan.Spec.Install.OtherAnsibleOpts["master_virtual_vip"] = curMasterHA.HAPrivateIP
		}
	}
	// for ansiblePlan add default value
	ansiblePlan = SetDefaultVars(&ansiblePlan)
	return ansiblePlan

}
// SetDefaultVars for ansiblePlan add default value
func SetDefaultVars(ansible *ecnsv1.AnsiblePlan) ecnsv1.AnsiblePlan  {
	// If vars key not found in list
	// Set value is item.value
	var DEFAULT  = map[string]string{
		"charts_repo_ip": "10.222.255.253",
		"cloud_provider": "external",
		"container_lvm_enabled": "false",
		"data_dir": "/kubernetes",
		"dnscache_enabled": "true",
		"docker_repo_enabled": "false",
		"epel_enabled": "false",
		"etcd_data_dir": "/kubernetes/etcd",
		"fs_server": "10.20.0.2",
		"fs_server_ip": "''",
		"helm_enabled": "true",
		"istio_enabled": "false",
		"kubeadm_enabled": "false",
		"kubepods_reserve": "true",
		"openstack_password": "Default",
		"openstack_project_domain_name": "Default",
		"openstack_project_name": "Default",
		"openstack_region_name": "Default",
		"openstack_user_app_cred_name": "Default",
		"openstack_user_name": "Default",
		"populate_inventory_to_hosts_file": "false",
		"preinstall_selinux_state": "disabled",
		"upstream_nameservers": "114.114.114.114",
		"vip_mgmt": "192.168.23.2",
		"webhook_enabled": "true",
		"flannel_interface": "eth0",
		"keepalived_interface": "eth0",
		"openstack_auth_domain": "keystone.openstack.svc.cluster.local",
		"openstack_cinder_domain": "cinder.openstack.svc.cluster.local",
		"openstack_nova_domain": "nova.openstack.svc.cluster.local",
		"harbor_admin_password": "cY4EMha0EIpDA2cW",
		"harbor_domain": "hub.ecns.io",
		"nvidia_driver_install_container": "false",
		"nvidia_accelerator_enabled": "false",
	}

	// Check ansible.ansible.Spec.Install.OtherAnsibleOpts not contains DEFAULT's key
	// Or set default value fot item.
	for defaultKey, defaultValue := range DEFAULT {
		_,found:=ansible.Spec.Install.OtherAnsibleOpts[defaultKey]
		if !found {
			ansible.Spec.Install.OtherAnsibleOpts[defaultKey] = defaultValue
		}
	}
	return *ansible
}

// PatchAnsiblePlan makes patch request to the MachineSet object.
func PatchAnsiblePlan(ctx context.Context, cli client.Client, cur, mod *ecnsv1.AnsiblePlan) error {
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
	err = cli.SubResource("status").Patch(ctx, cur, patchObj)
	if err != nil {
		return fmt.Errorf("failed to patch AnsiblePlan object status %s/%s: %s", cur.Namespace, cur.Name, err)
	}
	// client patch ansible plan object
	err = cli.Patch(ctx, cur, patchObj)
	if err != nil {
		return fmt.Errorf("failed to patch AnsiblePlan object %s/%s: %s", cur.Namespace, cur.Name, err)
	}
	return nil
}
