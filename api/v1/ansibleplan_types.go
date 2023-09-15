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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AnsiblePlanSpec defines the desired state of AnsiblePlan
type AnsiblePlanSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Type ExecType `json:"type"`
	// NodePools are the node pools
	Install *AnsibleInstall `json:"install,omitempty"`
	// AutoRun is the flag to indicate the plan is auto run
	AutoRun bool `json:"autoRun"`
	// ClusterName is the cluster name
	ClusterName string `json:"clusterName"`
	// SSHSecret is the ssh secret name
	SSHSecret string `json:"sshSecret"`
	// Version is the version of the k8s
	Version string `json:"version"`
	// SupportPython3 is the flag to indicate the Host support python3(default python is python3)
	SupportPython3 bool `json:"supportPython3"`
	// Execute ansible plan max retry times.
	MaxRetryTime int `json:"maxRetryTime"`
}

type AnsibleInstall struct {
	// NodePools are the node pools,we need print the config
	// like this:
	//# cat /etc/ansible/hosts
	NodePools []*AnsibleNode `json:"nodePools,omitempty"`
	//Bastion
	Bastion *AnsibleNode `json:"bastion"`
	// Etcd is the etcd group
	Etcd []string `json:"etcd,omitempty"`
	// KubeMaster is the kube master group
	KubeMaster []string `json:"kubeMaster,omitempty"`
	// KubeNode is the kube node group
	KubeNode []string `json:"kubeNode,omitempty"`
	// KubeIngress is the kube ingress group
	KubeIngress []string `json:"kubeIngress,omitempty"`
	// KubePrometheus is the kube prometheus group
	KubePrometheus []string `json:"kubePrometheus,omitempty"`
	// KubeLog is the kube log group
	KubeLog []string `json:"kubeLog,omitempty"`
	// OtherGroup is the other group
	OtherGroup map[string][]string `json:"otherGroup,omitempty"`
	// OtherAnsibleOpts is the ansible custome vars
	// OtherAnsibleOpts => ansible test/vars.yaml
	OtherAnsibleOpts map[string]string `json:"other_ansible_opts,omitempty"`
}

type AnsibleNode struct {
	// Name is the name of the node
	Name string `json:"name,omitempty"`
	// AnsibleHost is the ansible host
	AnsibleHost string `json:"ansibleHost,omitempty"`
	// AnsibleIP is the ansible ip
	AnsibleIP string `json:"ansibleIP,omitempty"`
	// MemoryReserve is the memory reserve(GB),default is -4,always < 0.
	MemoryReserve int64 `json:"memoryReserve,omitempty"`
	// AnsibleSSHPrivateKeyFile is the ansible ssh private key file
	AnsibleSSHPrivateKeyFile string `json:"ansibleSSHPrivateKeyFile,omitempty"`
}

// AnsiblePlanStatus defines the observed state of AnsiblePlan
type AnsiblePlanStatus struct {
	// Done is the flag to indicate the plan is done,which is an antiPattern.if Done is true,don't reconcile again
	// unless the plan operator is to take the initiative in changing the variable
	Done bool `json:"done"`
	// Times of ansible plan has been executed.
	CurrentRetryTime int `json:"currentRetryTime"`
}

type ProcessStatus struct {
	// ProcessPID is the linux  pid
	ProcessPID *int32 `json:"processPID,omitempty"`
	// ProcessStatus is the linux  process status
	ProcessStatus PIDStatus `json:"processStatus,omitempty"`
	// Reason is the reason of the process status
	Reason string `json:"reason,omitempty"`
}
type PIDStatus string

const (
	// PIDStatusRunning is the process running
	PIDStatusRunning PIDStatus = "running"
	// PIDStatusStop is the process stop
	PIDStatusStop PIDStatus = "stopped"
	// PIDStatusError is the process error
	PIDStatusError PIDStatus = "error"
)

type ExecType string

const (
	// ExecTypeInstall is the install type
	ExecTypeInstall ExecType = "install"
	// ExecTypeRemove is the remove node type
	ExecTypeRemove ExecType = "remove"
	// ExecTypeUpgrade is the upgrade type
	ExecTypeUpgrade ExecType = "upgrade"
	// ExecTypeExpansion is the up scale type
	ExecTypeExpansion ExecType = "expansion"
	// ExecTypeReset is the reset type
	ExecTypeReset ExecType = "reset"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// AnsiblePlan is the Schema for the ansibleplans API
type AnsiblePlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AnsiblePlanSpec   `json:"spec,omitempty"`
	Status AnsiblePlanStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AnsiblePlanList contains a list of AnsiblePlan
type AnsiblePlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AnsiblePlan `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AnsiblePlan{}, &AnsiblePlanList{})
}
