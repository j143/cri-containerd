/* Copyright 2017 The Kubernetes Authors.
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
	"testing"

	"github.com/containerd/containerd/api/services/containers"
	snapshotapi "github.com/containerd/containerd/api/services/snapshot"
	"github.com/containerd/containerd/api/types/mount"
	"github.com/containerd/containerd/api/types/task"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	runtime "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1"

	"github.com/kubernetes-incubator/cri-containerd/pkg/metadata"
	ostesting "github.com/kubernetes-incubator/cri-containerd/pkg/os/testing"
	servertesting "github.com/kubernetes-incubator/cri-containerd/pkg/server/testing"
)

func TestRemovePodSandbox(t *testing.T) {
	testID := "test-id"
	testName := "test-name"
	testMetadata := metadata.SandboxMetadata{
		ID:   testID,
		Name: testName,
	}
	for desc, test := range map[string]struct {
		sandboxTasks       []task.Task
		injectMetadata     bool
		removeSnapshotErr  error
		deleteContainerErr error
		taskInfoErr        error
		injectFSErr        error
		expectErr          bool
		expectRemoved      string
		expectCalls        []string
	}{
		"should not return error if sandbox does not exist": {
			injectMetadata:     false,
			removeSnapshotErr:  servertesting.SnapshotNotExistError,
			deleteContainerErr: servertesting.ContainerNotExistError,
			expectErr:          false,
			expectCalls:        []string{},
		},
		"should not return error if snapshot does not exist": {
			injectMetadata:    true,
			removeSnapshotErr: servertesting.SnapshotNotExistError,
			expectRemoved:     getSandboxRootDir(testRootDir, testID),
			expectCalls:       []string{"info"},
		},
		"should return error if remove snapshot fails": {
			injectMetadata:    true,
			removeSnapshotErr: fmt.Errorf("arbitrary error"),
			expectErr:         true,
			expectCalls:       []string{"info"},
		},
		"should return error when sandbox container task is not deleted": {
			injectMetadata: true,
			sandboxTasks:   []task.Task{{ID: testID}},
			expectErr:      true,
			expectCalls:    []string{"info"},
		},
		"should return error when arbitrary containerd error is injected": {
			injectMetadata: true,
			taskInfoErr:    fmt.Errorf("arbitrary error"),
			expectErr:      true,
			expectCalls:    []string{"info"},
		},
		"should return error when error fs error is injected": {
			injectMetadata: true,
			injectFSErr:    fmt.Errorf("fs error"),
			expectRemoved:  getSandboxRootDir(testRootDir, testID),
			expectErr:      true,
			expectCalls:    []string{"info"},
		},
		"should not return error if sandbox container does not exist": {
			injectMetadata:     true,
			deleteContainerErr: servertesting.ContainerNotExistError,
			expectRemoved:      getSandboxRootDir(testRootDir, testID),
			expectCalls:        []string{"info"},
		},
		"should return error if delete sandbox container fails": {
			injectMetadata:     true,
			deleteContainerErr: fmt.Errorf("arbitrary error"),
			expectRemoved:      getSandboxRootDir(testRootDir, testID),
			expectErr:          true,
			expectCalls:        []string{"info"},
		},
		"should be able to successfully delete": {
			injectMetadata: true,
			expectRemoved:  getSandboxRootDir(testRootDir, testID),
			expectCalls:    []string{"info"},
		},
	} {
		t.Logf("TestCase %q", desc)
		c := newTestCRIContainerdService()
		fake := c.containerService.(*servertesting.FakeContainersClient)
		fakeOS := c.os.(*ostesting.FakeOS)
		fakeExecutionClient := c.taskService.(*servertesting.FakeExecutionClient)
		fakeSnapshotClient := WithFakeSnapshotClient(c)
		fakeExecutionClient.SetFakeTasks(test.sandboxTasks)
		if test.injectMetadata {
			c.sandboxNameIndex.Reserve(testName, testID)
			c.sandboxIDIndex.Add(testID)
			c.sandboxStore.Create(testMetadata)
		}
		if test.removeSnapshotErr == nil {
			fakeSnapshotClient.SetFakeMounts(testID, []*mount.Mount{
				{
					Type:   "bind",
					Source: "/test/source",
					Target: "/test/target",
				},
			})
		} else {
			fakeSnapshotClient.InjectError("remove", test.removeSnapshotErr)
		}
		if test.deleteContainerErr == nil {
			_, err := fake.Create(context.Background(), &containers.CreateContainerRequest{
				Container: containers.Container{ID: testID},
			})
			assert.NoError(t, err)
		} else {
			fake.InjectError("delete", test.deleteContainerErr)
		}
		if test.taskInfoErr != nil {
			fakeExecutionClient.InjectError("info", test.taskInfoErr)
		}
		fakeOS.RemoveAllFn = func(path string) error {
			assert.Equal(t, test.expectRemoved, path)
			return test.injectFSErr
		}
		res, err := c.RemovePodSandbox(context.Background(), &runtime.RemovePodSandboxRequest{
			PodSandboxId: testID,
		})
		assert.Equal(t, test.expectCalls, fakeExecutionClient.GetCalledNames())
		if test.expectErr {
			assert.Error(t, err)
			assert.Nil(t, res)
			continue
		}
		assert.NoError(t, err)
		assert.NotNil(t, res)
		assert.NoError(t, c.sandboxNameIndex.Reserve(testName, testID),
			"sandbox name should be released")
		_, err = c.sandboxIDIndex.Get(testID)
		assert.Error(t, err, "sandbox id should be removed")
		meta, err := c.sandboxStore.Get(testID)
		assert.Error(t, err)
		assert.True(t, metadata.IsNotExistError(err))
		assert.Nil(t, meta, "sandbox metadata should be removed")
		mountsResp, err := fakeSnapshotClient.Mounts(context.Background(), &snapshotapi.MountsRequest{Key: testID})
		assert.Equal(t, servertesting.SnapshotNotExistError, err, "snapshot should be removed")
		assert.Nil(t, mountsResp)
		getResp, err := fake.Get(context.Background(), &containers.GetContainerRequest{ID: testID})
		assert.Equal(t, servertesting.ContainerNotExistError, err, "containerd container should be removed")
		assert.Nil(t, getResp)
		res, err = c.RemovePodSandbox(context.Background(), &runtime.RemovePodSandboxRequest{
			PodSandboxId: testID,
		})
		assert.NoError(t, err)
		assert.NotNil(t, res, "remove should be idempotent")
	}
}