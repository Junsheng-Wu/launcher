/*
Copyright 2020 The Kubernetes Authors.

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

package provider

import (
	"context"
	"fmt"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ecnsv1 "easystack.com/plan/api/v1"
	capoprovider "github.com/easystack/cluster-api-provider-openstack/pkg/cloud/services/provider"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/tokens"
	"github.com/gophercloud/utils/openstack/clientconfig"
)

// NewClientFromPlan token Auth form plan.spec.User get Client
func NewClientFromPlan(ctx context.Context, cli client.Client, plan *ecnsv1.Plan) (*gophercloud.ProviderClient, *clientconfig.ClientOpts, string, string, error) {
	if !plan.Spec.UserInfo.AuthSecretRef.IsEmpty() {
		cloud, _, err := capoprovider.GetCloudFromSecret(ctx, cli, plan.Spec.UserInfo.AuthSecretRef.NameSpace, plan.Spec.UserInfo.AuthSecretRef.Name, plan.Spec.ClusterName)
		if err != nil {
			return nil, nil, "", "", err
		}
		return NewClientWithUserID(cloud, nil)
	} else {
		cloud := getCloudFromPlan(plan)
		return NewClientWithUserID(cloud, nil)
	}
}

func NewClientWithUserID(cloud capoprovider.NewCloud, caCert []byte) (*gophercloud.ProviderClient, *clientconfig.ClientOpts, string, string, error) {
	provider, clientOpts, projectID, err := capoprovider.NewClient(cloud, caCert)
	if err != nil {
		return nil, nil, "", "", err
	}
	userID, err := getUserIDFromAuthResult(provider.GetAuthResult())
	if err != nil {
		return nil, nil, "", "", err
	}

	return provider, clientOpts, projectID, userID, nil
}

// getCloudFromPlan extract a Cloud from the given plan.
func getCloudFromPlan(plan *ecnsv1.Plan) capoprovider.NewCloud {
	return capoprovider.NewCloud{
		RegionName:         plan.Spec.UserInfo.Region,
		IdentityAPIVersion: "3",
		AuthType:           "token",
		AuthInfo: &capoprovider.NewAuthInfo{
			AuthInfo: clientconfig.AuthInfo{
				AuthURL: plan.Spec.UserInfo.AuthUrl,
				Token:   plan.Spec.UserInfo.Token}}}
}

// getUserIDFromAuthResult handles different auth mechanisms to retrieve the
// current user id. Usually we use the Identity v3 Token mechanism that
// returns the user id in the response to the initial auth request.
func getUserIDFromAuthResult(authResult gophercloud.AuthResult) (string, error) {
	switch authResult := authResult.(type) {
	case tokens.CreateResult:
		user, err := authResult.ExtractUser()
		if err != nil {
			return "", fmt.Errorf("unable to extract user from CreateResult: %v", err)
		}

		return user.ID, nil
	case tokens.GetResult:
		user, err := authResult.ExtractUser()
		if err != nil {
			return "", fmt.Errorf("unable to extract user from CreateResult: %v", err)
		}
		return user.ID, nil
	default:
		return "", fmt.Errorf("unable to get the user id from auth response with type %T", authResult)
	}
}
