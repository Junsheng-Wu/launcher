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

package v1

import (
	clusteropenstack "github.com/easystack/cluster-api-provider-openstack/api/v1alpha6"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
const (
	// MachineFinalizer allows ReconcileOpenStackMachine to clean up OpenStack resources associated with OpenStackMachine before
	// removing it from the apiserver.
	MachineFinalizer = "plan.ecns.easystack.com"

	ClusterFinalizer = "cluster.cluster.x-k8s.io"

	// MachineSetClusterLabelName is the cluster label name
	MachineSetClusterLabelName = "cluster.x-k8s.io/cluster-name"

	// MachineSetLabelName is the machine set label name
	MachineSetLabelName = "cluster.x-k8s.io/set-name"

	AnsibleFinalizer = "ansible.ecns.easystack.com"

	// MachineControlPlaneLabelName is the label set on machines or related objects that are part of a control plane.
	MachineControlPlaneLabelName = "cluster.x-k8s.io/control-plane"
)

type SetRole string

const (
	MasterSetRole     = "master"
	WorkSetRole       = "node"
	PrometheusSetRole = "prometheus"
	IngressSetRole    = "ingress"
	LogSetRole        = "log"
	Etcd              = "etcd"
)

type NetWorkMode string

const (
	NetWorkExist = "existed"
	NetWorkNew   = "new"
)

// PlanSpec defines the desired state of Plan
type PlanSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// NetMode is an flag to indicate the mode of the plan
	// one is  use existed network, the other is created new network.
	// value is "existed" or "new"

	NetMode NetWorkMode `json:"mode"`

	//LBEnable is the flag to decide to create lb or no
	LBEnable bool `json:"lb_enable"`

	// K8sVersion is the version of kubernetes => ansible kubernetes tag
	K8sVersion string `json:"k8sVersion"`

	// SupportPython3 is the flag to decide to ansible use python3 version
	SupportPython3 bool `json:"support_python3"`

	// ClusterName is the cluster name => clusters.cluster.x-k8s.io
	// --------------------------------=> openstackclusters.infrastructure.cluster.x-k8s.io
	ClusterName string `json:"cluster_name"`

	// SshKey is the all cluster machine's public key
	// Maybe we should create a new ssh pair for every cluster
	// SshKey is the public key of the pair
	SshKey string `json:"ssh_key"`

	// UseFloatIP decied to get fips or no
	UseFloatIP bool `json:"use_float_ip"`

	// externalNetworkId is the external network id
	// WHEN use_float_ip is true, we will get a fip from this network
	ExternalNetworkId string `json:"external_network_id,omitempty"`

	// DNSNameservers is the dns nameservers of subnet which auto created
	DNSNameservers []string `json:"dns_nameservers,omitempty"`

	// NodeCIDR is the node cidr of subnet which NetworkMode is new
	NodeCIDR string `json:"node_cidr,omitempty"`

	// NeedKeepAlive is the flag to decide to keep alive the machine_sets role
	NeedKeepAlive []string `json:"need_keep_alive"`

	// NeedLoadBalancer is the flag to decide to create loadBalancer
	NeedLoadBalancer []string `json:"need_load_balancer"`

	MachineSets []*MachineSetReconcile `json:"machine_sets"`

	// Monitor is the pvc config of etcd
	Monitor MonitorConfig `json:"monitor"`

	// CniType is the cni type
	CniType string `json:"cni_type"`

	// CniWorkMode is the cni work mode
	CniWorkMode string `json:"cni_work_mode,omitempty"`

	// PodCidr is the pod cidr
	PodCidr string `json:"pod_cidr,omitempty"`

	//SvcCidr is the svc cidr
	SvcCidr string `json:"svc_cidr,omitempty"`

	// OtherAnsibleOpts is the ansible custome vars
	// OtherAnsibleOpts => ansible test/vars.yaml
	OtherAnsibleOpts map[string]string `json:"other_ansible_opts,omitempty"`

	//Paused is the flag to pause the plan
	Paused bool `json:"paused,omitempty"`

	// AnsiblePlanAuto  decide to auto to run ansible plan
	AnsiblePlanAuto bool `json:"ansible_plan_auto,omitempty"`

	// UserInfo is the user of keystone auth
	UserInfo User `json:"user,omitempty"`

	// Execute ansible plan max retry times.
	MaxRetryTime int `json:"maxRetryTime"`
}

// User is the user of keystone auth
// include AuthUrl
// include Token
// include Region
type User struct {
	// AuthUrl is the auth url of keystone
	AuthUrl string `json:"auth_url"`
	// Token is the token of keystone,expired time is 6h
	Token string `json:"token"`
	// Region is the region of keystone
	Region string `json:"region"`
}

// MonitorConfig is the monitor other config
// include pvc cap
// include pvc type
// include auto clear days
type MonitorConfig struct {
	// PvcType is the pvc type
	PvcType string `json:"pvc_type"`
	// PvcCap is the pvc cap
	PvcCap string `json:"pvc_cap"`
	// AutoClearDays is the auto clear days
	AutoClearDays string `json:"auto_clear_days"`
}

// MachineSetReconcile is the  machine config
// Maybe we should create a Bastion machine for master machine to access
// and set the SSH  rsa  to the Bastion machine
type MachineSetReconcile struct {
	// Name is the name of machine
	Name string `json:"name"`
	// Number is the number of all machine
	Number int32 `json:"number"`
	// Role is the role of machine
	Role string `json:"role"`
	// Infras is the infras of machine
	Infra []*Infras `json:"infras,omitempty"`
	// CloudInit is the cloud init secret of machine,base64 file,can use it to config the machine
	// such as init disk...
	CloudInit string `json:"init,omitempty"`
}
type Infras struct {
	// UID is the uid of infra
	UID string `json:"uid"`
	// AvailabilityZone are a set of failure domains for machines
	// decide the every machine's AZ
	AvailabilityZone string `json:"availability_zone"`
	// subnets are a set of subnets for machines
	// decide the every machine's subnet
	// when NetMode == existed, the subnets is required
	Subnets *Subnet `json:"subnets,omitempty"`
	// Volumes are the volume type of machine,include root volume and data volume
	Volumes []*volume `json:"volumes,omitempty"`
	// image is the image of machine
	Image string `json:"image"`
	// Flavor is the flavor of machine
	Flavor string `json:"flavor"`
	// replica is the replica of machine
	Replica int32 `json:"replica"`
}
type volume struct {
	// VolumeType is the volume type of machine
	VolumeType string `json:"volume_type"`
	// VolumeSize is the volume size of machine
	VolumeSize int `json:"volume_size"`
	// Index is the index of volume 0==root volume
	Index int `json:"index"`
}
type Subnet struct {
	// SubnetNetwork is the network of subnet
	SubnetNetwork string `json:"subnet_network"`
	// uuid is the subnet uuid of subnet
	SubnetUUID string `json:"subnet_uuid"`
	// FixIP is the fix ip of subnet
	FixIP string `json:"fix_ip"`
}

// PlanStatus defines the observed state of Plan
type PlanStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	//ServerGroupID is the server group id of cluster
	ServerGroupID *Servergroups `json:"server_group_id,omitempty"`
	// VMDone show the vm is created or not
	VMDone bool `json:"vm_done,omitempty"`
	// OpenstackMachineList is the list of openstack machine
	OpenstackMachineList []clusteropenstack.OpenStackMachine `json:"openstack_machine_list,omitempty"`
	// InfraMachine is the list of infra machine,key is set role name,value is the InfraMachine
	InfraMachine map[string]InfraMachine `json:"infra_machine,omitempty"`
	// PlanLoadBalancer is the list of load balancer of plan
	PlanLoadBalancer []*LoadBalancer `json:"planLoadBalancer,omitempty"`
	// Bastion is the bastion of plan
	Bastion *clusteropenstack.Instance `json:"bastion,omitempty"`
}

// LoadBalancer represents basic information about the associated OpenStack LoadBalancer.
type LoadBalancer struct {
	Name       string `json:"name"`
	ID         string `json:"id"`
	IP         string `json:"ip"`
	InternalIP string `json:"internalIP"`
	//+optional
	AllowedCIDRs []string `json:"allowedCIDRs,omitempty"`
}

type InfraMachine struct {
	// Role is the role of machine
	Role string `json:"role,omitempty"`
	// PortIDs is the port id of machines
	PortIDs []string `json:"port_ids,omitempty"`
	// IPs is the ips of machine,key is the instance name(openstackMachine name),value is the ip
	IPs map[string]string `json:"ips,omitempty"`
	// HAPortID is the port id of HA
	HAPortID string `json:"ha_port_id,omitempty"`
	// HAPrivateIP is the ip of HA
	HAPrivateIP string `json:"ha_private_ip,omitempty"`
	// HAPublicIP is the public ip of HA
	HAPublicIP string `json:"ha_public_ip,omitempty"`
}

type Servergroups struct {
	// MasterServerGroupID is the server group id of master machine
	MasterServerGroupID string `json:"master_server_group_id,omitempty"`
	// WorkerServerGroupID is the server group id of worker machine
	WorkerServerGroupID string `json:"worker_server_group_id,omitempty"`
}

type MachineSetStatus struct {
	// name is the name of machineset
	Name string `json:"name"`
	// ready is the number of ready machine
	Ready int `json:"ready"`
	// replicasnoew is the number of replicas machines
	ReplicasNow int `json:"replicas_now"`
	// available is the number of available machine
	Available int `json:"available"`
	// ReadyMachines is the ready machine list
	ReadyMachines []MachineStatus `json:"ready_machines"`
}

type MachineStatus struct {
	// name is the name of machine
	Name   string                                   `json:"name,omitempty"`
	Status *clusteropenstack.OpenStackMachineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Plan is the Schema for the plans API
type Plan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PlanSpec   `json:"spec,omitempty"`
	Status PlanStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PlanList contains a list of Plan
type PlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Plan `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Plan{}, &PlanList{})
}
