package utils

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"text/template"

	ecnsv1 "easystack.com/plan/api/v1"
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
[log]
{{range .KubeLog}}
{{.}}
{{end}}
{{range $key, $value := .OtherGroup}}
[{{$key}}]
{{range $value}}
{{.}}
{{end}}
{{end}}
`

const AnsibleVars = `
node_resources:
  {{range .Install.NodePools}}
  {{.Name}}: {memory: {{.MemoryReserve}}}
  {{end}}
hyperkube_image_tag: {{.Version}}
{{range $key, $value := .Install.OtherAnsibleOpts}}
{{$key}}: {{$value}}
{{end}}
`

func GetOrCreateInventoryFile(ctx context.Context, cli client.Client, ansible *ecnsv1.AnsiblePlan) error {
	t, err := template.New("inventory").Parse(AnsibleInventory)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	// Execute the template and write the output to the file
	err = t.Execute(&buf, ansible.Spec.Install)
	if err != nil {
		return err
	}
	// get ansible cr uuid,create inventory file by uid
	File := fmt.Sprintf("/opt/captain/inventory/%s", ansible.UID)
	if FileExist(File) {
		//delete this path file
		err = os.RemoveAll(File)
		if err != nil {
			// 删除失败
			return err
		}
	}
	//create file
	f, err := os.Create(File)
	if err != nil {
		return err
	}
	_, err = f.Write(buf.Bytes())
	if err != nil {
		return err
	}
	return nil
}

func GetOrCreateVarsFile(ctx context.Context, cli client.Client, ansible *ecnsv1.AnsiblePlan) error {
	t, err := template.New("vars").Parse(AnsibleVars)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	// Execute the template and write the output to the file
	err = t.Execute(&buf, ansible.Spec)
	if err != nil {
		return err
	}
	// get ansible cr uuid,create inventory file by uid
	File := fmt.Sprintf("/opt/captain/test/%s.vars", ansible.UID)
	if FileExist(File) {
		//delete this path file
		err = os.RemoveAll(File)
		if err != nil {
			// 删除失败
			return err
		}
	}
	//create file
	f, err := os.Create(File)
	if err != nil {
		return err
	}
	_, err = f.Write(buf.Bytes())
	if err != nil {
		return err
	}
	return nil
}

//	TODO start ansible plan
//
// 1.EXEC ANSIBLE COMMAND
// 2.print ansible log to one log file by ansible cr uid(one cluster only has one log file)
// 3.WAIT ANSIBLE task RETURN RESULT AND UPDATE ANSIBLE CR DONE=TRUE
func StartAnsiblePlan(ctx context.Context, cli client.Client, ansible *ecnsv1.AnsiblePlan) error {
	var playbook string
	if ansible.Spec.Type == ecnsv1.ExecTypeInstall {
		playbook = "cluster.yml"
	}
	if ansible.Spec.Type == ecnsv1.ExecTypeExpansion {
		playbook = "scale.yml"
	}
	if ansible.Spec.Type == ecnsv1.ExecTypeUpgrade {
		playbook = "upgrade-cluster.yml"
	}
	if ansible.Spec.Type == ecnsv1.ExecTypeRemove {
		playbook = "remove-node-eks.yml"
	}
	if ansible.Spec.Type == ecnsv1.ExecTypeReset {
		playbook = "reset.yml"
	}
	var inventory = fmt.Sprintf("/opt/captain/inventory/%s", ansible.UID)
	cmd := exec.Command("ansible-playbook", "-i", inventory, playbook, "--extra-vars", "@"+fmt.Sprintf("/opt/captain/test/%s.vars", ansible.UID), "-vvv")
	// TODO cmd.Dir need to be change when python version change.
	if ansible.Spec.SupportPython3 {
		cmd.Dir = "/opt/captain3"
	} else {
		cmd.Dir = "/opt/captain"
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var logfile = fmt.Sprintf("/tmp/%s.log", ansible.UID)
	var logFile *os.File
	defer logFile.Close()
	if !FileExist(logfile) {
		// if log file not exist,create it and get io.writer
		logFile, err = os.Create(logfile)
		if err != nil {
			return err
		}

	} else {
		// if log file exist,open it and get io.writer
		logFile, err = os.OpenFile(logfile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
	}
	var stdoutCopy = bufio.NewWriter(logFile)
	_, err = io.Copy(stdoutCopy, stdout)
	if err != nil {
		return err
	}
	if err = cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			log.Printf("Exit Status: %v, For more logs please check the file %s", exiterr, logfile)
		} else {
			log.Fatalf("cmd.Wait: %v", err)
		}
		stdoutCopy.Flush()
		return err
	}
	stdoutCopy.Flush()

	return nil
}
