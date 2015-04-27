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
	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecstcs"
	"github.com/aws/amazon-ecs-agent/agent/api"
	ecsengine "github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	mock_resolver "github.com/aws/amazon-ecs-agent/agent/stats/resolver/mock"
)

var defaultCluster string
var defaultContainerInstance string

func init() {
	defaultCluster = "default"
	defaultContainerInstance = "ci"
}

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

func (engine *MockTaskEngine) AddTask(*api.Task) error {
	return nil
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

func (engine *MockTaskEngine) Disable() {
}

func validateContainerMetrics(containerMetrics []*ecstcs.ContainerMetric, expected int) error {
	if len(containerMetrics) != expected {
		return fmt.Errorf("Mismatch in number of ContainerStatsSet elements. Expected: %d, Got: %d", expected, len(containerMetrics))
	}
	for _, containerMetric := range containerMetrics {
		if containerMetric.CpuStatsSet == nil {
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
	resolver := mock_resolver.NewMockContainerMetadataResolver(mockCtrl)
	t1 := &api.Task{Arn: "t1", Family: "f1"}
	t2 := &api.Task{Arn: "t2", Family: "f2"}
	t3 := &api.Task{Arn: "t3"}
	resolver.EXPECT().ResolveTask("c1").AnyTimes().Return(t1, nil)
	resolver.EXPECT().ResolveTask("c2").AnyTimes().Return(t1, nil)
	resolver.EXPECT().ResolveTask("c3").AnyTimes().Return(t2, nil)
	resolver.EXPECT().ResolveTask("c4").AnyTimes().Return(nil, errors.New("unmapped container"))
	resolver.EXPECT().ResolveTask("c5").AnyTimes().Return(t2, nil)
	resolver.EXPECT().ResolveTask("c6").AnyTimes().Return(t3, nil)

	resolver.EXPECT().ResolveName("c1").AnyTimes().Return("n-c1", nil)
	resolver.EXPECT().ResolveName("c2").AnyTimes().Return("n-c2", nil)
	resolver.EXPECT().ResolveName("c3").AnyTimes().Return("n-c3", nil)
	resolver.EXPECT().ResolveName("c4").AnyTimes().Return("", errors.New("unmapped container"))
	resolver.EXPECT().ResolveName("c5").AnyTimes().Return("", errors.New("unmapped container"))
	resolver.EXPECT().ResolveName("c6").AnyTimes().Return("n-c6", nil)

	engine := NewDockerStatsEngine()
	engine.resolver = resolver
	engine.metricsMetadata = newMetricsMetadata(&defaultCluster, &defaultContainerInstance)

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

	metadata, taskMetrics, err := engine.GetInstanceMetrics()
	if err != nil {
		t.Error("Error gettting instance metrics: ", err)
	}

	if metadata == nil {
		t.Fatal("Metadata is nil")
	}
	if *metadata.Cluster != defaultCluster {
		t.Error("Expected cluster in metadata to be: ", defaultCluster, " got: ", *metadata.Cluster)
	}
	if *metadata.ContainerInstance != defaultContainerInstance {
		t.Error("Expected container instance in metadata to be: ", defaultContainerInstance, " got: ", *metadata.ContainerInstance)
	}

	if len(taskMetrics) != 1 {
		t.Error("Incorrect number of tasks. Expected: 1, got: ", len(taskMetrics))
	}
	err = validateContainerMetrics(taskMetrics[0].ContainerMetrics, 2)
	if err != nil {
		t.Error("Error validating container metrics: ", err)
	}
	if *taskMetrics[0].TaskArn != "t1" {
		t.Error("Incorrect task arn. Expected: t1, got: ", *taskMetrics[0].TaskArn)
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

	_, _, err = engine.GetInstanceMetrics()
	if err == nil {
		t.Error("Expected non-empty error for empty stats.")
	}
	engine.RemoveContainer("c3")

	// Should get an error while adding this container due to unmapped
	// container to task.
	engine.AddContainer("c4")
	_, _, err = engine.GetInstanceMetrics()
	if err == nil {
		t.Error("Expected non-empty error for empty stats.")
	}

	// Should get an error while adding this container due to unmapped
	// container to name.
	engine.AddContainer("c5")
	_, _, err = engine.GetInstanceMetrics()
	if err == nil {
		t.Error("Expected non-empty error for empty stats.")
	}

	// Should get an error while adding this container due to unmapped
	// task arn to task definition family.
	engine.AddContainer("c6")
	_, _, err = engine.GetInstanceMetrics()
	if err == nil {
		t.Error("Expected non-empty error for empty stats.")
	}
}

func TestStatsEngineMetadataInStatsSets(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	resolver := mock_resolver.NewMockContainerMetadataResolver(mockCtrl)
	t1 := &api.Task{Arn: "t1", Family: "f1"}
	resolver.EXPECT().ResolveTask("c1").AnyTimes().Return(t1, nil)
	resolver.EXPECT().ResolveName("c1").AnyTimes().Return("n-c1", nil)

	engine := NewDockerStatsEngine()
	engine.resolver = resolver
	engine.metricsMetadata = newMetricsMetadata(&defaultCluster, &defaultContainerInstance)
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
	metadata, taskMetrics, err := engine.GetInstanceMetrics()
	if err != nil {
		t.Error("Error gettting instance metrics: ", err)
	}
	if len(taskMetrics) != 1 {
		t.Fatal("Incorrect number of tasks. Expected: 1, got: ", len(taskMetrics))
	}
	err = validateContainerMetrics(taskMetrics[0].ContainerMetrics, 1)
	if err != nil {
		t.Error("Error validating container metrics: ", err)
	}
	if *taskMetrics[0].TaskArn != "t1" {
		t.Error("Incorrect task arn. Expected: t1, got: ", *taskMetrics[0].TaskArn)
	}
	if *metadata.Cluster != defaultCluster {
		t.Errorf("Cluster Arn not set in metadata. Expected: %s, got: %s", defaultCluster, *metadata.Cluster)
	}
	if *metadata.ContainerInstance != defaultContainerInstance {
		t.Errorf("Container Instance Arn not set in metadata. Expected: %s, got: %s", defaultContainerInstance, *metadata.ContainerInstance)
	}

	engine.RemoveContainer("c1")
	_, _, err = engine.GetInstanceMetrics()
	if err == nil {
		t.Error("Expected non-empty error for empty stats.")
	}
}

func TestStatsEngineInvalidTaskEngine(t *testing.T) {
	statsEngine := NewDockerStatsEngine()
	taskEngine := &MockTaskEngine{}
	err := statsEngine.MustInit(taskEngine, nil)
	if err == nil {
		t.Error("Expected error in engine initialization, got nil")
	}
}

func TestStatsEngineUninitialized(t *testing.T) {
	engine := NewDockerStatsEngine()
	engine.resolver = &DockerContainerMetadataResolver{}
	engine.AddContainer("c1")
	_, _, err := engine.GetInstanceMetrics()
	if err == nil {
		t.Error("Expected non-empty error for empty stats.")
	}
}

func TestStatsEngineTerminalTask(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	resolver := mock_resolver.NewMockContainerMetadataResolver(mockCtrl)
	resolver.EXPECT().ResolveTask("c1").Return(&api.Task{Arn: "t1", KnownStatus: api.TaskStopped}, nil)
	engine := NewDockerStatsEngine()
	engine.resolver = resolver

	engine.AddContainer("c1")
	_, _, err := engine.GetInstanceMetrics()
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
	if !IsMetricCollectionEnabled() {
		t.Error("Stats engine disabled when ECS_DISABLE_METRICS is not set")
	}
	os.Setenv("ECS_DISABLE_METRICS", "opinion")
	setMetricCollectionFlag()
	if !IsMetricCollectionEnabled() {
		t.Error("Stats engine disabled when ECS_DISABLE_METRICS is neither true nor false")
	}
	os.Setenv("ECS_DISABLE_METRICS", "false")
	setMetricCollectionFlag()
	if !IsMetricCollectionEnabled() {
		t.Error("Stats engine disabled when ECS_DISABLE_METRICS is false")
	}
	os.Setenv("ECS_DISABLE_METRICS", "true")
	setMetricCollectionFlag()
	if IsMetricCollectionEnabled() {
		t.Error("Stats engine enabled when ECS_DISABLE_METRICS is true")
	}
}
