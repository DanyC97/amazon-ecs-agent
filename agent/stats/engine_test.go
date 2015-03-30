// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package stats

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"code.google.com/p/gomock/gomock"
	"github.com/aws/amazon-ecs-agent/agent/api"
	ecsengine "github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
)

const defaultClusterArn = "default"
const defaultContainerInstanceArn = "ci"

type MockTaskEngine struct {
}

func (engine *MockTaskEngine) Init() error {
	return nil
}
func (engine *MockTaskEngine) MustInit() {
}

func (engine *MockTaskEngine) TaskEvents() <-chan api.ContainerStateChange {
	return make(chan api.ContainerStateChange)
}

func (engine *MockTaskEngine) SetSaver(statemanager.Saver) {
}

func (engine *MockTaskEngine) AddTask(*api.Task) {
}

func (engine *MockTaskEngine) ListTasks() ([]*api.Task, error) {
	return nil, nil
}

func (engine *MockTaskEngine) UnmarshalJSON([]byte) error {
	return nil
}

func (engine *MockTaskEngine) MarshalJSON() ([]byte, error) {
	return make([]byte, 0), nil
}

func (engine *MockTaskEngine) Version() (string, error) {
	return "", nil
}

func validateContainerMetrics(containerMetrics []ContainerMetric, expected int) error {
	if len(containerMetrics) != expected {
		return fmt.Errorf("Mismatch in number of ContainerStatsSet elements. Expected: %d, Got: %d", expected, len(containerMetrics))
	}
	for _, containerMetric := range containerMetrics {
		if containerMetric.CPUStatsSet == nil {
			return fmt.Errorf("CPUStatsSet is nil")
		}
		if containerMetric.MemoryStatsSet == nil {
			return fmt.Errorf("MemoryStatsSet is nil")
		}
	}

	return nil
}

func TestStatsEngineAddRemoveContainers(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	resolver := NewMockContainerMetadataResolver(mockCtrl)
	resolver.EXPECT().ResolveTask("c1").AnyTimes().Return(&api.Task{Arn: "t1"}, nil)
	resolver.EXPECT().ResolveTask("c2").AnyTimes().Return(&api.Task{Arn: "t1"}, nil)
	resolver.EXPECT().ResolveTask("c3").AnyTimes().Return(&api.Task{Arn: "t2"}, nil)
	resolver.EXPECT().ResolveTask("c4").AnyTimes().Return(nil, errors.New("unmapped container"))
	resolver.EXPECT().ResolveTask("c5").AnyTimes().Return(&api.Task{Arn: "t2"}, nil)

	resolver.EXPECT().ResolveName("c1").AnyTimes().Return("n-c1", nil)
	resolver.EXPECT().ResolveName("c2").AnyTimes().Return("n-c2", nil)
	resolver.EXPECT().ResolveName("c3").AnyTimes().Return("n-c3", nil)
	resolver.EXPECT().ResolveName("c4").AnyTimes().Return("", errors.New("unmapped container"))
	resolver.EXPECT().ResolveName("c5").AnyTimes().Return("", errors.New("unmapped container"))

	engine := NewDockerStatsEngine()
	engine.resolver = resolver

	engine.AddContainer("c1")
	engine.AddContainer("c1")

	if len(engine.tasksToContainers) != 1 {
		t.Error("Adding containers failed. Expected num tasks = 1, got: ", len(engine.tasksToContainers))
	}

	containers, _ := engine.tasksToContainers["t1"]
	if len(containers) != 1 {
		t.Error("Adding duplicate containers failed.")
	}
	_, exists := containers["c1"]
	if !exists {
		t.Error("Container c1 not found in engine")
	}

	engine.AddContainer("c2")
	containers, _ = engine.tasksToContainers["t1"]
	_, exists = containers["c2"]
	if !exists {
		t.Error("Container c2 not found in engine")
	}

	containerStats := []*ContainerStats{
		CreateContainerStats(22400432, 1839104, ParseNanoTime("2015-02-12T21:22:05.131117533Z")),
		CreateContainerStats(116499979, 3649536, ParseNanoTime("2015-02-12T21:22:05.232291187Z")),
	}
	for _, cronContainer := range containers {
		for i := 0; i < 2; i++ {
			cronContainer.statsQueue.Add(containerStats[i])
		}
	}

	// Ensure task shows up in metrics.
	containerMetrics, err := engine.getContainerMetricsForTask("t1")
	if err != nil {
		t.Error("Error getting container metrics: ", err)
	}
	err = validateContainerMetrics(containerMetrics, 2)
	if err != nil {
		t.Error("Error validating container metrics: ", err)
	}
	instanceMetrics, err := engine.GetInstanceMetrics()
	if err != nil {
		t.Error("Error gettting instance metrics: ", err)
	}

	taskMetrics := instanceMetrics.TaskMetrics
	if len(taskMetrics) != 1 {
		t.Error("Incorrect number of tasks. Expected: 1, got: ", len(taskMetrics))
	}
	err = validateContainerMetrics(taskMetrics[0].ContainerMetrics, 2)
	if err != nil {
		t.Error("Error validating container metrics: ", err)
	}
	if taskMetrics[0].TaskArn != "t1" {
		t.Error("Incorrect task arn. Expected: t1, got: ", taskMetrics[0].TaskArn)
	}

	// Ensure that only valid task shows up in metrics.
	_, err = engine.getContainerMetricsForTask("t2")
	if err == nil {
		t.Error("Expected non-empty error for non existent task")
	}

	engine.RemoveContainer("c1")
	containers, _ = engine.tasksToContainers["t1"]
	_, exists = containers["c1"]
	if exists {
		t.Error("Container c1 not removed from engine")
	}
	engine.RemoveContainer("c2")
	containers, _ = engine.tasksToContainers["t1"]
	_, exists = containers["c2"]
	if exists {
		t.Error("Container c2 not removed from engine")
	}
	engine.AddContainer("c3")
	containers, _ = engine.tasksToContainers["t2"]
	_, exists = containers["c3"]
	if !exists {
		t.Error("Container c3 not found in engine")
	}

	_, err = engine.GetInstanceMetrics()
	if err == nil {
		t.Error("Expected non-empty error for empty stats.")
	}
	engine.RemoveContainer("c3")

	// Should get an error while adding this container due to unmapped
	// container to task.
	engine.AddContainer("c4")
	_, err = engine.GetInstanceMetrics()
	if err == nil {
		t.Error("Expected non-empty error for empty stats.")
	}

	// Should get an error while adding this container due to unmapped
	// container to name.
	engine.AddContainer("c5")
	_, err = engine.GetInstanceMetrics()
	if err == nil {
		t.Error("Expected non-empty error for empty stats.")
	}
}

func TestStatsEngineMetadataInStatsSets(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	resolver := NewMockContainerMetadataResolver(mockCtrl)
	resolver.EXPECT().ResolveTask("c1").AnyTimes().Return(&api.Task{Arn: "t1"}, nil)
	resolver.EXPECT().ResolveName("c1").AnyTimes().Return("n-c1", nil)

	engine := NewDockerStatsEngine()
	engine.resolver = resolver
	engine.instanceMetadata = newInstanceMetadata(defaultClusterArn, defaultContainerInstanceArn)
	engine.AddContainer("c1")
	containerStats := []*ContainerStats{
		CreateContainerStats(22400432, 1839104, ParseNanoTime("2015-02-12T21:22:05.131117533Z")),
		CreateContainerStats(116499979, 3649536, ParseNanoTime("2015-02-12T21:22:05.232291187Z")),
	}
	containers, _ := engine.tasksToContainers["t1"]
	for _, cronContainer := range containers {
		for i := 0; i < 2; i++ {
			cronContainer.statsQueue.Add(containerStats[i])
		}
	}
	instanceMetrics, err := engine.GetInstanceMetrics()
	if err != nil {
		t.Error("Error gettting instance metrics: ", err)
	}
	taskMetrics := instanceMetrics.TaskMetrics
	if len(taskMetrics) != 1 {
		t.Error("Incorrect number of tasks. Expected: 1, got: ", len(taskMetrics))
	}
	err = validateContainerMetrics(taskMetrics[0].ContainerMetrics, 1)
	if err != nil {
		t.Error("Error validating container metrics: ", err)
	}
	if taskMetrics[0].TaskArn != "t1" {
		t.Error("Incorrect task arn. Expected: t1, got: ", taskMetrics[0].TaskArn)
	}
	if instanceMetrics.Metadata.ClusterArn != defaultClusterArn {
		t.Errorf("Cluster Arn not set in metadata. Expected: %s, got: %s", defaultClusterArn, instanceMetrics.Metadata.ClusterArn)
	}
	if instanceMetrics.Metadata.ContainerInstanceArn != defaultContainerInstanceArn {
		t.Errorf("Container Instance Arn not set in metadata. Expected: %s, got: %s", defaultContainerInstanceArn, instanceMetrics.Metadata.ContainerInstanceArn)
	}

	engine.RemoveContainer("c1")
	_, err = engine.GetInstanceMetrics()
	if err == nil {
		t.Error("Expected non-empty error for empty stats.")
	}
}

func TestStatsEngineInvalidTaskEngine(t *testing.T) {
	statsEngine := NewDockerStatsEngine()
	taskEngine := &MockTaskEngine{}
	err := statsEngine.MustInit(taskEngine, "", "")
	if err == nil {
		t.Error("Expected error in engine initialization, got nil")
	}
}

func TestStatsEngineUninitialized(t *testing.T) {
	engine := NewDockerStatsEngine()
	engine.resolver = &DockerContainerMetadataResolver{}
	engine.AddContainer("c1")
	_, err := engine.GetInstanceMetrics()
	if err == nil {
		t.Error("Expected non-empty error for empty stats.")
	}
}

func TestStatsEngineTerminalTask(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	resolver := NewMockContainerMetadataResolver(mockCtrl)
	resolver.EXPECT().ResolveTask("c1").Return(&api.Task{Arn: "t1", KnownStatus: api.TaskStopped}, nil)
	engine := NewDockerStatsEngine()
	engine.resolver = resolver

	engine.AddContainer("c1")
	_, err := engine.GetInstanceMetrics()
	if err == nil {
		t.Error("Expected non-empty error for empty stats.")
	}
}

func TestStatsEngineClientErrorListingContainers(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	engine := NewDockerStatsEngine()
	mockDockerClient := NewMockDockerClient(mockCtrl)
	// Mock client will return error while listing images.
	mockDockerClient.EXPECT().ListContainers(false).Return(nil, errors.New("could not list containers"))
	engine.client = mockDockerClient
	mockChannel := make(chan ecsengine.DockerContainerChangeEvent)
	mockDockerClient.EXPECT().ContainerEvents().Return(mockChannel, nil, nil)
	mockDockerClient.EXPECT().UnsubscribeContainerEvents(gomock.Any()).Return(nil)
	engine.client = mockDockerClient
	engine.Init()

	time.Sleep(waitForCleanupSleep)
	// Make sure that the stats engine deregisters the event listener when it fails to
	// list images.
	if engine.dockerEventListener != nil {
		t.Error("Event listener hasn't been reset")
	}
}

func TestStatsEngineDisabledEnvVar(t *testing.T) {
	os.Unsetenv("ECS_DISABLE_METRICS")
	setMetricCollectionFlag()
	if IsMetricCollectionDisabled() {
		t.Error("Stats engine disabled when ECS_DISABLE_METRICS is not set")
	}
	os.Setenv("ECS_DISABLE_METRICS", "opinion")
	setMetricCollectionFlag()
	if IsMetricCollectionDisabled() {
		t.Error("Stats engine disabled when ECS_DISABLE_METRICS is neither true nor false")
	}
	os.Setenv("ECS_DISABLE_METRICS", "false")
	setMetricCollectionFlag()
	if IsMetricCollectionDisabled() {
		t.Error("Stats engine disabled when ECS_DISABLE_METRICS is false")
	}
	os.Setenv("ECS_DISABLE_METRICS", "true")
	setMetricCollectionFlag()
	if !IsMetricCollectionDisabled() {
		t.Error("Stats engine enabled when ECS_DISABLE_METRICS is true")
	}
}
