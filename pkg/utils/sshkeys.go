package utils

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"

	ecnsv1 "easystack.com/plan/api/v1"

	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const SSHSecretSuffix = "-default-ssh"

func MakeSSHKeyPair() (string, string, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return "", "", err
	}

	// generate and write private key as PEM
	var privKeyBuf strings.Builder
	privateKeyPEM := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}
	if err := pem.Encode(&privKeyBuf, privateKeyPEM); err != nil {
		return "", "", err
	}

	// generate and write public key
	pub, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", err
	}

	var pubKeyBuf strings.Builder
	pubKeyBuf.Write(ssh.MarshalAuthorizedKey(pub))

	return pubKeyBuf.String(), privKeyBuf.String(), nil
}

func GetOrCreateSSHKeySecret(ctx context.Context, client client.Client, plan *ecnsv1.Plan) (string, string, error) {
	secretName := fmt.Sprintf("%s%s", plan.Name, SSHSecretSuffix)
	//get secret by name secretName
	secret := &corev1.Secret{}
	err := client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: plan.Namespace}, secret)
	if err != nil {
		// if err is not found, create secret
		if apierrors.IsNotFound(err) {
			pub, pri, err := MakeSSHKeyPair()
			if err != nil {
				return "", "", err
			}
			secret.Namespace = plan.Namespace
			secret.Data = map[string][]byte{
				"public_key":  []byte(pub),
				"private_key": []byte(pri),
			}
			secret.Name = secretName
			err = client.Create(ctx, secret)
			if err != nil {
				return pub, pri, err
			}
			return pub, pri, nil
		}
		return "", "", err
	}
	// if secret is found, return public key and private key
	pub := string(secret.Data["public_key"])
	pri := string(secret.Data["private_key"])
	return pub, pri, nil
}

// func GetSecretByName(ctx context.Context, client client.Client, secretName string, namespace string) (string, string, error) {
// 	secret := &corev1.Secret{}
// 	err := client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
// 	if err != nil {
// 		return "", "", err
// 	}
// 	pub := string(secret.Data["public_key"])
// 	pri := string(secret.Data["private_key"])
// 	if pri == "" {
// 		return "", "", errors.New("private key is empty,please check it")
// 	}
// 	return pub, pri, nil
// }

// // GetOrCreateSSHkeyFile  create private key file
// func GetOrCreateSSHkeyFile(ctx context.Context, cli client.Client, ansible *ecnsv1.AnsiblePlan) error {
// 	path := fmt.Sprintf("/root/.ssh/id_rsa_%s", ansible.Spec.ClusterName)
// 	// judge if path of file exists
// 	if FileExist(path) {
// 		// delete file
// 		err := os.RemoveAll(path)
// 		if err != nil {
// 			return err
// 		}
// 	}
// 	// get public key and private key
// 	_, pri, err := GetSecretByName(context.Background(), cli, ansible.Spec.SSHSecret, ansible.Namespace)
// 	if err != nil {
// 		return err
// 	}

// 	// create file
// 	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
// 	if err != nil {
// 		return err
// 	}

// 	defer file.Close()

// 	_, err = file.Write([]byte(pri))
// 	if err != nil {
// 		return err
// 	}

// 	for i, pool := range ansible.Spec.Install.NodePools {
// 		if pool.AnsibleSSHPrivateKeyFile == "" {
// 			ansible.Spec.Install.NodePools[i].AnsibleSSHPrivateKeyFile = path
// 		}
// 	}
// 	ansible.Spec.Install.OtherAnsibleOpts["ansible_user"] = "root"
// 	ansible.Spec.Install.OtherAnsibleOpts["ansible_ssh_private_key_file"] = path
// 	return nil
// }

// func FileExist(path string) bool {
// 	_, err := os.Lstat(path)
// 	return !os.IsNotExist(err)
// }
