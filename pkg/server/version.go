/*
Copyright 2017 The Kubernetes Authors.

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

package server

import (
	"fmt"

	"golang.org/x/net/context"
	"k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

const (
	containerName        = "containerd"
	containerdAPIVersion = "0.0.0"
	// kubeAPIVersion is the api version of kubernetes.
	kubeAPIVersion = "0.1.0"
)

// Version returns the runtime name, runtime version and runtime API version.
func (c *criContainerdService) Version(ctx context.Context, r *runtime.VersionRequest) (*runtime.VersionResponse, error) {
	resp, err := c.client.Version(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get containerd version: %v", err)
	}
	return &runtime.VersionResponse{
		Version:        kubeAPIVersion,
		RuntimeName:    containerName,
		RuntimeVersion: resp.Version,
		// Containerd doesn't have an api version now.
		RuntimeApiVersion: containerdAPIVersion,
	}, nil
}
