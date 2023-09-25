package utils

import (
	"bytes"
	"context"
	ecnsv1 "easystack.com/plan/api/v1"
	"easystack.com/plan/pkg/cloud/service/provider"
	"easystack.com/plan/pkg/cloudinit"
	"easystack.com/plan/pkg/scope"
	"encoding/base64"
	"encoding/json"
	errNew "errors"
	"fmt"
	clusteropenstack "github.com/easystack/cluster-api-provider-openstack/api/v1alpha6"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/jsonmergepatch"
	"k8s.io/utils/pointer"
	clusterapi "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"text/template"
	"time"
)

const (
	LabelTemplateInfra          = "easystack.com/infra"
	LabelEasyStackCluster       = "easystack.com/cluster"
	LabelEasyStackPlan          = "easystack.com/plan"
	Clusterapibootstrapapi      = "bootstrap.cluster.x-k8s.io/v1beta1"
	Clusterapibootstrapkind     = "KubeadmConfigTemplate"
	Clusteropenstackapi         = "infrastructure.cluster.x-k8s.io/v1alpha6"
	Clusteropenstackkind        = "OpenStackMachineTemplate"
	CloudInitSecretSuffix       = "-cloudinit"
	retryIntervalInstanceStatus = 2 * time.Second
	timeoutInstanceCreate       = 20 * time.Second

	// OpenstackGlobalAuthTpl go template
	OpenstackGlobalAuthTpl = `[Global]
auth-url={{.AuthInfo.AuthURL}}
application-credential-id={{.AuthInfo.ApplicationCredentialID}}
application-credential-secret={{.AuthInfo.ApplicationCredentialSecret}}
region={{.RegionName}}
[BlockStorage]
bs-version=v2
ignore-volume-az=True
`
)

type NewPartition struct {
	// Device is the name of the device.
	Device string `json:"device"`
	// Layout specifies the device layout.
	// If it is true, a single partition will be created for the entire device.
	// When layout is false, it means don't partition or ignore existing partitioning.
	Layout []string `json:"layout"`
	// Overwrite describes whether to skip checks and create the partition if a partition or filesystem is found on the device.
	// Use with caution. Default is 'false'.
	// +optional
	Overwrite *bool `json:"overwrite,omitempty"`
	// TableType specifies the tupe of partition table. The following are supported:
	// 'mbr': default and setups a MS-DOS partition table
	// 'gpt': setups a GPT partition table
	// +optional
	TableType *string `json:"tableType,omitempty"`
}

func ListMachineSets(ctx context.Context, cli client.Client, plan *ecnsv1.Plan) (clusterapi.MachineSetList, error) {
	var msetList clusterapi.MachineSetList
	labels := map[string]string{ecnsv1.MachineSetClusterLabelName: plan.Spec.ClusterName}
	cli.List(ctx, &msetList, client.InNamespace(plan.Namespace), client.MatchingLabels(labels))
	return msetList, nil
}

// CreateMachineSet when create machineset, we need to finish the following things:
// 1. check openstack cluster ready
// 2. get or create openstacktemplate resource with plan MachineSetReconcile first element(eg AvailabilityZone,Subnets,FloatingIPPool,Volumes)
// 3. get or create cloud init secret
// 4.create a new machineset replicas==0,deletePolicy==Newest
func CreateMachineSet(ctx context.Context, scope *scope.Scope, client client.Client, plan *ecnsv1.Plan, set *ecnsv1.MachineSetReconcile, masterGroupID string, nodeGroupID string) error {
	clusterReady, err := checkOpenstackClusterReady(ctx, client, plan)
	if err != nil {
		return err
	}
	if !clusterReady {
		return errNew.New("openstack cluster is not ready")
	}
	err = getOrCreateOpenstackTemplate(ctx, scope, client, plan, set, 0, masterGroupID, nodeGroupID)
	if err != nil {
		return err
	}
	err = getOrCreateCloudInitSecret(ctx, scope, client, plan, set)
	if err != nil {
		return err
	}
	err = createMachineset(ctx, scope, client, plan, set, 0)

	if err != nil {
		return err
	}
	return nil
}

// AddReplicas for machineset add replicas
func AddReplicas(ctx context.Context, scope *scope.Scope, cli client.Client, target *ecnsv1.MachineSetReconcile, setName string, plan *ecnsv1.Plan, index int, mastergroup string, nodegroup string, Mre int32) error {
	err := getOrCreateOpenstackTemplate(ctx, scope, cli, plan, target, index, mastergroup, nodegroup)
	if err != nil {
		return err
	}
	err = getOrCreateCloudInitSecret(ctx, scope, cli, plan, target)
	if err != nil {
		return err
	}

	if index > len(target.Infra) {
		scope.Logger.Error(fmt.Errorf("index out of range infra"), "check plan machinesetreconcile infra")
		return errNew.New("index out of range infra")
	}
	infra := target.Infra[index]

	var actual clusterapi.MachineSet
	err = cli.Get(ctx, types.NamespacedName{Name: setName, Namespace: plan.Namespace}, &actual)
	if err != nil {
		return err
	}
	// merge patch machineSet config
	origin := actual.DeepCopy()
	fakeOrigin := actual.DeepCopy()
	var replicas int32 = Mre
	actual.Spec.Replicas = &replicas
	scope.Logger.Info("old machineSet  replicas", "replicas", origin.Spec.Replicas)
	scope.Logger.Info("prepare add to replicas", "replicas", replicas)
	actual.Spec.Template.Spec.FailureDomain = &infra.AvailabilityZone
	actual.Spec.Template.Spec.InfrastructureRef.APIVersion = Clusteropenstackapi
	actual.Spec.Template.Spec.InfrastructureRef.Kind = Clusteropenstackkind
	actual.Spec.Template.Spec.InfrastructureRef.Name = fmt.Sprintf("%s%s%s", plan.Spec.ClusterName, target.Role, infra.UID)
	err = PatchMachineSet(ctx, cli, origin, &actual)
	if err != nil {
		return err
	}
	//TODO check new machine has created and InfrastructureRef !=nil,or give a reason to user
	err = PollImmediate(retryIntervalInstanceStatus, timeoutInstanceCreate, func() (bool, error) {
		var m clusterapi.MachineSet
		err = cli.Get(ctx, types.NamespacedName{
			Namespace: actual.Namespace,
			Name:      actual.Name,
		}, &m)
		if err != nil {
			return false, err
		}
		scope.Logger.Info("wait add to replicas", "replicas", m.Status.FullyLabeledReplicas)

		scope.Logger.Info("in fact", "replicas", m.Status.FullyLabeledReplicas)
		scope.Logger.Info("target fact", "replicas", *fakeOrigin.Spec.Replicas+1)

		switch m.Status.FullyLabeledReplicas {
		case *fakeOrigin.Spec.Replicas + 1:
			return true, nil
		default:
			return false, nil
		}
	})
	if err != nil {
		return fmt.Errorf("add check replicas is ready error:%v in get replicas %d", err, *actual.Spec.Replicas)
	}
	scope.Logger.Info("add replicas success", "replicas", actual.Spec.Replicas)
	return nil
}

// SubReplicas for machineSet sub replicas
func SubReplicas(ctx context.Context, scope *scope.Scope, cli client.Client, target *ecnsv1.MachineSetReconcile, setName string, plan *ecnsv1.Plan, index int, Mre int32) error {
	if index > len(target.Infra) {
		scope.Logger.Error(fmt.Errorf("index out of range infra"), "check plan machinesetreconcile infra")
		return errNew.New("index out of range infra")
	}
	infra := target.Infra[index]

	var actual clusterapi.MachineSet
	err := cli.Get(ctx, types.NamespacedName{Name: setName, Namespace: plan.Namespace}, &actual)
	if err != nil {
		return err
	}
	// merge patch machineSet config
	origin := actual.DeepCopy()
	fakeOrigin := actual.DeepCopy()
	var replicas int32 = Mre
	actual.Spec.Replicas = &replicas
	actual.Spec.Template.Spec.FailureDomain = &infra.AvailabilityZone
	actual.Spec.Template.Spec.InfrastructureRef.APIVersion = Clusteropenstackapi
	actual.Spec.Template.Spec.InfrastructureRef.Kind = Clusteropenstackkind
	actual.Spec.Template.Spec.InfrastructureRef.Name = fmt.Sprintf("%s%s%s", plan.Spec.ClusterName, target.Role, infra.UID)
	err = PatchMachineSet(ctx, cli, origin, &actual)
	if err != nil {
		return err
	}
	//TODO check new machine has created and InfrastructureRef !=nil,or give a reason to user
	err = PollImmediate(retryIntervalInstanceStatus, timeoutInstanceCreate, func() (bool, error) {
		var m clusterapi.MachineSet
		err = cli.Get(ctx, types.NamespacedName{
			Namespace: actual.Namespace,
			Name:      actual.Name,
		}, &m)
		if err != nil {
			return false, err
		}
		scope.Logger.Info("in fact", "replicas", m.Status.FullyLabeledReplicas)
		scope.Logger.Info("target fact", "replicas", *fakeOrigin.Spec.Replicas-1)
		switch m.Status.FullyLabeledReplicas {
		case *fakeOrigin.Spec.Replicas - 1:
			return true, nil
		default:
			return false, nil
		}
	})
	if err != nil {
		return fmt.Errorf("sub check replicas is ready error:%v in get replicas %d", err, *actual.Spec.Replicas)
	}
	scope.Logger.Info("add replicas success", "replicas", actual.Spec.Replicas)
	return nil
}

// PatchMachineSet makes patch request to the MachineSet object.
func PatchMachineSet(ctx context.Context, client client.Client, cur, mod *clusterapi.MachineSet) error {
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
	patchObj := runtimeclient.RawPatch(types.MergePatchType, patch)
	// client patch machineSet replicas
	err = client.Patch(ctx, cur, patchObj)
	if err != nil {
		return fmt.Errorf("failed to patch MachineSet object %s/%s: %s", cur.Namespace, cur.Name, err)
	}

	return nil
}

// TODO check openstack cluster ready,one openstack cluster for one plan
func checkOpenstackClusterReady(ctx context.Context, client client.Client, plan *ecnsv1.Plan) (bool, error) {
	var cluster clusteropenstack.OpenStackCluster
	err := client.Get(ctx, types.NamespacedName{
		Namespace: plan.Namespace,
		Name:      plan.Spec.ClusterName,
	}, &cluster)
	if err != nil {
		return false, err
	}
	if cluster.Status.Ready && (cluster.Status.Bastion != nil && cluster.Status.Bastion.State == clusteropenstack.InstanceStateActive) {
		return true, nil
	}
	return false, nil
}

func CheckOpenstackClusterReady(ctx context.Context, client client.Client, plan *ecnsv1.Plan) (bool, error) {
	return checkOpenstackClusterReady(ctx, client, plan)
}

// TODO get or create openstacktemplate resource,n openstacktemplate for one machineset
func getOrCreateOpenstackTemplate(ctx context.Context, scope *scope.Scope, client client.Client, plan *ecnsv1.Plan, set *ecnsv1.MachineSetReconcile, index int, masterGroup string, nodeGroup string) (rer error) {

	if index > len(set.Infra)-1 {
		scope.Logger.Error(fmt.Errorf("index out of range infra"), "check plan machinesetreconcile infra")
		return errNew.New("index out of range infra")
	}
	infra := set.Infra[index]
	// get openstacktemplate by name ,if not exist,create it
	var openstackTemplate clusteropenstack.OpenStackMachineTemplate
	// get openstacktemplate by filiter from cache
	err := client.Get(ctx, types.NamespacedName{
		Namespace: plan.Namespace,
		Name:      fmt.Sprintf("%s%s%s", plan.Spec.ClusterName, set.Role, infra.UID),
	}, &openstackTemplate)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// create openstacktemplate
			// add label to openstacktemplate with infra uid
			openstackTemplate.ObjectMeta.Labels = map[string]string{
				LabelTemplateInfra:    infra.UID,
				LabelEasyStackCluster: plan.Spec.ClusterName,
			}
			openstackTemplate.Name = fmt.Sprintf("%s%s%s", plan.Spec.ClusterName, set.Role, infra.UID)
			openstackTemplate.Namespace = plan.Namespace
			openstackTemplate.Spec.Template.Spec.Flavor = infra.Flavor
			openstackTemplate.Spec.Template.Spec.Image = infra.Image
			openstackTemplate.Spec.Template.Spec.SSHKeyName = plan.Spec.SshKey
			openstackTemplate.Spec.Template.Spec.CloudName = plan.Spec.ClusterName
			openstackTemplate.Spec.Template.Spec.IdentityRef = &clusteropenstack.OpenStackIdentityReference{}
			secretName := fmt.Sprintf("%s-%s", plan.Spec.ClusterName, "admin-etc")
			openstackTemplate.Spec.Template.Spec.IdentityRef.Kind = "Secret"
			openstackTemplate.Spec.Template.Spec.IdentityRef.Name = secretName
			openstackTemplate.Spec.Template.Spec.RootVolume = &clusteropenstack.RootVolume{}
			for _, volume := range infra.Volumes {
				if volume.Index == 1 {
					openstackTemplate.Spec.Template.Spec.RootVolume.VolumeType = volume.VolumeType
					openstackTemplate.Spec.Template.Spec.RootVolume.Size = volume.VolumeSize
				} else {
					openstackTemplate.Spec.Template.Spec.CustomeVolumes = append(openstackTemplate.Spec.Template.Spec.CustomeVolumes, &clusteropenstack.RootVolume{
						Size:       volume.VolumeSize,
						VolumeType: volume.VolumeType,
					})
				}
			}
			if plan.Spec.NetMode == "existed" {
				if infra.Subnets != nil {
					if infra.Replica > 1 && infra.Subnets.FixIP != "" {
						err = errors.NewBadRequest("replica more than 1,fixIp must be empty")
						scope.Logger.Error(fmt.Errorf("replica more than 1,fixip must be empty"), "please check your plan machineSetReconcile infra subnets fixIp")
						return err
					}
					if infra.Subnets.SubnetUUID == "" {
						err = errors.NewBadRequest("subnet uuid is empty")
						scope.Logger.Error(err, "please check your plan machineSetReconcile infra subnets uuid")
						return err
					} else {
						openstackTemplate.Spec.Template.Spec.Ports = append(openstackTemplate.Spec.Template.Spec.Ports, clusteropenstack.PortOpts{
							Network: &clusteropenstack.NetworkFilter{
								ID: infra.Subnets.SubnetNetwork,
							},
							FixedIPs: []clusteropenstack.FixedIP{
								{
									Subnet: &clusteropenstack.SubnetFilter{
										ID: infra.Subnets.SubnetUUID,
									},
									IPAddress: infra.Subnets.FixIP,
								},
							},
						})
					}

				} else {
					err = errors.NewBadRequest("subnet  is empty")
					scope.Logger.Error(err, "please check your plan machineSetReconcile infra subnets")
					return err

				}
			} else {
				// dont config subnet
			}

			if set.Role == "master" {
				openstackTemplate.Spec.Template.Spec.ServerGroupID = masterGroup
			} else {
				openstackTemplate.Spec.Template.Spec.ServerGroupID = nodeGroup
			}
			//TODO create openstacktemplate resource
			err = client.Create(ctx, &openstackTemplate)
			if err != nil {
				return err
			}
			return nil
		}
		return err

	}
	return nil
}

func GetOrCreateCloudInitSecret(ctx context.Context, scope *scope.Scope, client client.Client, plan *ecnsv1.Plan, set *ecnsv1.MachineSetReconcile) error {
	return getOrCreateCloudInitSecret(ctx, scope, client, plan, set)
}

// TODO get or create cloud init secret,one cloud init secret for one machineset
func getOrCreateCloudInitSecret(ctx context.Context, scope *scope.Scope, client client.Client, plan *ecnsv1.Plan, set *ecnsv1.MachineSetReconcile) error {
	// get cloud init secret by name ,if not exist,create it
	var cloudInitSecret corev1.Secret
	secretName := fmt.Sprintf("%s-%s%s", plan.Spec.ClusterName, set.Name, CloudInitSecretSuffix)
	err := client.Get(ctx, types.NamespacedName{
		Namespace: plan.Namespace,
		Name:      secretName,
	}, &cloudInitSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// create cloud init secret
			cloudInitSecret.Name = secretName
			cloudInitSecret.Namespace = plan.Namespace
			cloudInitSecret.Data = make(map[string][]byte)
			// TODO add cloud init
			// 1. add ssh key
			// 2. add openstack app cre secret
			// 3. add init disk script
			// 4. add set cloud init

			// 1. add asnible  ssh key
			var sshKey corev1.Secret
			err = client.Get(ctx, types.NamespacedName{
				Namespace: plan.Namespace,
				Name:      fmt.Sprintf("%s%s", plan.Name, SSHSecretSuffix),
			}, &sshKey)
			if err != nil {
				return err
			}

			var eksInput cloudinit.EKSInput
			sshBase64 := base64.StdEncoding.EncodeToString(sshKey.Data["public_key"])
			eksInput.WriteFiles = append(eksInput.WriteFiles, bootstrapv1.File{
				Path:        "/root/.ssh/authorized_keys",
				Owner:       "root:root",
				Permissions: "0644",
				Encoding:    bootstrapv1.Base64,
				Append:      true,
				Content:     sshBase64,
			})
			// 2. add openstack app cre secret
			var openstackAppCreSecret corev1.Secret
			err = client.Get(ctx, types.NamespacedName{
				Namespace: plan.Namespace,
				Name:      fmt.Sprintf("%s-%s", plan.Spec.ClusterName, "admin-etc"),
			}, &openstackAppCreSecret)
			if err != nil {
				return err
			}
			cloud, _, err := provider.GetCloudFromSecret(ctx, client, plan.Namespace, fmt.Sprintf("%s-%s", plan.Spec.ClusterName, "admin-etc"), plan.Spec.ClusterName)
			if err != nil {
				return err
			}
			OpenstackTmpl, err := template.New("openstack").Parse(OpenstackGlobalAuthTpl)
			if err != nil {
				return err
			}
			var authBuf bytes.Buffer
			err = OpenstackTmpl.Execute(&authBuf, cloud)
			if err != nil {
				return err
			}

			authBase64 := base64.StdEncoding.EncodeToString(authBuf.Bytes())
			eksInput.WriteFiles = append(eksInput.WriteFiles, bootstrapv1.File{
				Path:        "/opt/cloud_config",
				Owner:       "root:root",
				Permissions: "0644",
				Encoding:    bootstrapv1.Base64,
				Append:      false,
				Content:     authBase64,
			})

			// 3. add init disk script

			var tableType string = "gpt"
			// all node  need init /kubernetes/ path,https://easystack.atlassian.net/wiki/spaces/delivery/pages/1929052161/EKS+-k8s-v1.26#6.5-%E7%AE%A1%E7%90%86%E8%8A%82%E7%82%B9%E6%8C%82%E8%BD%BD%E5%8D%B7https://easystack.atlassian.net/wiki/spaces/delivery/pages/1929052161/EKS+-k8s-v1.26#6.5-%E7%AE%A1%E7%90%86%E8%8A%82%E7%82%B9%E6%8C%82%E8%BD%BD%E5%8D%B7https://easystack.atlassian.net/wiki/spaces/delivery/pages/1929052161/EKS+-k8s-v1.26#6.5-%E7%AE%A1%E7%90%86%E8%8A%82%E7%82%B9%E6%8C%82%E8%BD%BD%E5%8D%B7https://easystack.atlassian.net/wiki/spaces/delivery/pages/1929052161/EKS+-k8s-v1.26#6.5-%E7%AE%A1%E7%90%86%E8%8A%82%E7%82%B9%E6%8C%82%E8%BD%BD%E5%8D%B7
			eksInput.DiskSetup = &bootstrapv1.DiskSetup{
				Partitions: []bootstrapv1.Partition{
					{
						Device:    "/dev/vdb",
						TableType: &tableType,
						Layout:    true,
						Overwrite: pointer.Bool(false),
					},
					{
						Device:    "/dev/vdc",
						TableType: &tableType,
						Layout:    true,
						Overwrite: pointer.Bool(false),
					},
				},
				Filesystems: []bootstrapv1.Filesystem{
					{
						Device:     "/dev/vdb",
						Filesystem: "xfs",
						Partition:  pointer.String("auto"),
						Overwrite:  pointer.Bool(false),
						Label:      "kubernetes",
					},
					{
						Device:     "/dev/vdc",
						Filesystem: "xfs",
						Overwrite:  pointer.Bool(false),
						Partition:  pointer.String("auto"),
						Label:      "data",
					},
				},
			}
			eksInput.Mounts = append(eksInput.Mounts, bootstrapv1.MountPoints{"/dev/vdb1", "/kubernetes"})
			eksInput.Mounts = append(eksInput.Mounts, bootstrapv1.MountPoints{"/dev/vdc1", "/data"})

			// add sleep 10s command,make sure disk init success
			eksInput.PreKubeadmCommands = append(eksInput.PreKubeadmCommands, "sh -c 'sleep 10'")

			cloudInitData, err := cloudinit.NewEKS(&eksInput)
			if err != nil {
				return err
			}

			if set.CloudInit != "" {

				cloudInitData = []byte(fmt.Sprintf("%s\n%s ", string(cloudInitData), set.CloudInit))
			}
			cloudInitSecret.Data["format"] = []byte("cloud-config")
			cloudInitSecret.Data["value"] = cloudInitData
			//TODO create cloud init secret resource
			err = client.Create(ctx, &cloudInitSecret)
			if err != nil {
				return err
			}
			return nil

		}
		return err
	}
	return nil
}

// TODO create a new machineset replicas==0,deletePolicy==Newest,one machineset for one plan.spec.machinesetsReconcile
func createMachineset(ctx context.Context, scope *scope.Scope, client client.Client, plan *ecnsv1.Plan, set *ecnsv1.MachineSetReconcile, index int) error {
	infra := set.Infra[index]
	var machineSet clusterapi.MachineSet
	machineSet.Name = fmt.Sprintf("%s%s", plan.Spec.ClusterName, set.Role)
	machineSet.Namespace = plan.Namespace
	machineSet.Spec.ClusterName = plan.Spec.ClusterName
	var re int32 = 0
	machineSet.Spec.Replicas = &re
	machineSet.Spec.DeletePolicy = "Newest"
	machineSet.Spec.Selector.MatchLabels = make(map[string]string)
	machineSet.Spec.Selector.MatchLabels["cluster.x-k8s.io/cluster-name"] = plan.Spec.ClusterName
	if plan.Spec.UseFloatIP == true && set.Role == ecnsv1.MasterSetRole {
		machineSet.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
		machineSet.Spec.Template.ObjectMeta.Annotations["machinedeployment.clusters.x-k8s.io/fip"] = "enable"
	}
	machineSet.Spec.Template.Labels = make(map[string]string)
	machineSet.Spec.Template.Labels["cluster.x-k8s.io/cluster-name"] = plan.Spec.ClusterName
	if set.Role == ecnsv1.MasterSetRole || set.Role == ecnsv1.Etcd {
		machineSet.Spec.Template.Labels[ecnsv1.MachineControlPlaneLabelName] = "true"
	}
	machineSet.Spec.Template.Spec.Bootstrap.ConfigRef = &corev1.ObjectReference{}
	machineSet.Spec.Template.Spec.Bootstrap.ConfigRef.APIVersion = Clusterapibootstrapapi
	machineSet.Spec.Template.Spec.Bootstrap.ConfigRef.Kind = Clusterapibootstrapkind
	machineSet.Spec.Template.Spec.Bootstrap.ConfigRef.Name = plan.Spec.ClusterName
	cloud_secret_name := fmt.Sprintf("%s-%s-cloudinit", plan.Spec.ClusterName, set.Name)
	machineSet.Spec.Template.Spec.Bootstrap.DataSecretName = &cloud_secret_name
	machineSet.Spec.Template.Spec.ClusterName = plan.Spec.ClusterName
	machineSet.Spec.Template.Spec.FailureDomain = &infra.AvailabilityZone
	machineSet.Spec.Template.Spec.InfrastructureRef.APIVersion = Clusteropenstackapi
	machineSet.Spec.Template.Spec.InfrastructureRef.Kind = Clusteropenstackkind
	machineSet.Spec.Template.Spec.InfrastructureRef.Name = fmt.Sprintf("%s%s%s", plan.Spec.ClusterName, set.Role, infra.UID)
	machineSet.Spec.Template.Spec.Version = &plan.Spec.K8sVersion
	err := client.Create(ctx, &machineSet)
	if err != nil {
		return err
	}
	return nil
}
