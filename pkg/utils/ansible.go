package utils

import (
	"bytes"
	"context"
	"text/template"

	ecnsv1 "easystack.com/plan/api/v1"
	kubeanapis "github.com/kubean-io/kubean-api/apis"
	clusterv1alpha1 "github.com/kubean-io/kubean-api/apis/cluster/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const AnsibleInventory = `## Configure 'ip' variable to bind kubernetes services on a
## different ip than the default iface
{{range .NodePools}}
{{.Name}}  ansible_ssh_host={{.AnsibleHost}} ansible_ssh_private_key_file={{.AnsibleSSHPrivateKeyFile}}  ip={{.AnsibleIP}} ansible_user=root
{{end}}
bastion ansible_ssh_host={{.Bastion.AnsibleHost}} ansible_host={{.Bastion.AnsibleHost}}
[kube-master]
{{range .KubeMaster}}
{{.}}
{{end}}
[etcd]
{{range .Etcd}}
{{.}}
{{end}}
[kube-node]
{{range .KubeNode}}
{{.}}
{{end}}
[k8s-cluster:children]
etcd
kube-master
kube-node
[ingress]
{{range .KubeIngress}}
{{.}}
{{end}}
[prometheus]
{{range .KubePrometheus}}
{{.}}
{{end}}
{{range $key, $value := .ExtendGroups}}
[{{$key}}]
{{range $value}}
{{.}}
{{end}}
{{end}}
`

const AnsibleVars = `
node_resources:
  {{range .HostConf.NodePools}}
  {{.Name}}: {memory: {{.MemoryReserve}}}
  {{end}}
kube_version: {{.K8sVersion}}
{{range $key, $value := .OtherAnsibleOpts}}
{{$key}}: {{$value}}
{{end}}
`

func CreateHostConfConfigMap(ctx context.Context, cli client.Client, plan *ecnsv1.Plan) (*corev1.ConfigMap, error) {
	t, err := template.New("hostConf").Parse(AnsibleInventory)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	// Execute the template and write the output to the file
	err = t.Execute(&buf, plan.Spec.HostConf)
	if err != nil {
		return nil, err
	}

	config := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: plan.Namespace,
			Name:      plan.Spec.HostConfName,
		},
		Data: map[string]string{
			"hosts.yml": buf.String(),
		},
	}

	return config, nil
}

func CreateVarsConfConfigMap(ctx context.Context, cli client.Client, plan *ecnsv1.Plan) (*corev1.ConfigMap, error) {
	t, err := template.New("vars").Parse(AnsibleVars)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	// Execute the template and write the output to the file
	err = t.Execute(&buf, plan.Spec)
	if err != nil {
		return nil, err
	}

	config := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: plan.Namespace,
			Name:      plan.Spec.VarsConfName,
		},
		Data: map[string]string{
			"group_vars.yml": buf.String(),
		},
	}
	// get ansible cr uuid, create inventory file by uid

	return config, nil
}

func CreateOpsCluster(plan *ecnsv1.Plan) *clusterv1alpha1.Cluster {
	cluster := &clusterv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: plan.Namespace,
			Name:      plan.Spec.ClusterName,
		},
		Spec: clusterv1alpha1.Spec{
			HostsConfRef: &kubeanapis.DataRef{
				NameSpace: plan.Namespace,
				Name:      plan.Spec.HostConfName,
			},
			VarsConfRef: &kubeanapis.DataRef{
				NameSpace: plan.Namespace,
				Name:      plan.Spec.VarsConfName,
			},
			SSHAuthRef: &kubeanapis.DataRef{
				NameSpace: plan.Namespace,
				Name:      plan.Spec.SshKey,
			},
		},
	}
	return cluster
}
