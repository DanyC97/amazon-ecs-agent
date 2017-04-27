// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package api

import (
	"reflect"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/fsouza/go-dockerclient"
	"github.com/stretchr/testify/assert"
)

func TestOverridden(t *testing.T) {
	container := &Container{
		Name:                "name",
		Image:               "image",
		Command:             []string{"foo", "bar"},
		CPU:                 1,
		Memory:              1,
		Links:               []string{},
		Ports:               []PortBinding{PortBinding{10, 10, "", TransportProtocolTCP}},
		Overrides:           ContainerOverrides{},
		DesiredStatusUnsafe: ContainerRunning,
		AppliedStatus:       ContainerRunning,
		KnownStatusUnsafe:   ContainerRunning,
	}

	overridden := container.Overridden()
	// No overrides, should be identity
	assert.True(t, reflect.DeepEqual(container, overridden))
	assert.Equal(t, container, overridden)
	overridden.Name = "mutated"
	assert.Equal(t, container.Name, "name", "Should make a copy")
}

type configPair struct {
	Container *Container
	Config    *docker.Config
}

func (pair configPair) Equal() bool {
	conf := pair.Config
	cont := pair.Container

	if (conf.Memory / 1024 / 1024) != int64(cont.Memory) {
		return false
	}
	if conf.CPUShares != int64(cont.CPU) {
		return false
	}
	if conf.Image != cont.Image {
		return false
	}
	if cont.EntryPoint == nil && !utils.StrSliceEqual(conf.Entrypoint, []string{}) {
		return false
	}
	if cont.EntryPoint != nil && !utils.StrSliceEqual(conf.Entrypoint, *cont.EntryPoint) {
		return false
	}
	if !utils.StrSliceEqual(cont.Command, conf.Cmd) {
		return false
	}
	// TODO, Volumes, VolumesFrom, ExposedPorts

	return true
}

func TestGetSteadyStateStatusReturnsRunningByDefault(t *testing.T) {
	container := &Container{}
	assert.Equal(t, container.GetSteadyStateStatus(), ContainerRunning)
}

func TestIsKnownSteadyState(t *testing.T) {
	container := &Container{}
	assert.False(t, container.IsKnownSteadyState())
	container.SetKnownStatus(ContainerCreated)
	assert.False(t, container.IsKnownSteadyState())
	container.SetKnownStatus(ContainerRunning)
	assert.True(t, container.IsKnownSteadyState())
	resourcesProvisioned := ContainerResourcesProvisioned
	container.steadyState = &resourcesProvisioned
	assert.False(t, container.IsKnownSteadyState())
	container.SetKnownStatus(ContainerResourcesProvisioned)
	assert.True(t, container.IsKnownSteadyState())
}

func TestGetNextStateProgression(t *testing.T) {
	container := &Container{}
	assert.Equal(t, container.GetNextKnownStateProgression(), ContainerPulled)
	container.SetKnownStatus(ContainerPulled)
	assert.Equal(t, container.GetNextKnownStateProgression(), ContainerCreated)
	container.SetKnownStatus(ContainerCreated)
	assert.Equal(t, container.GetNextKnownStateProgression(), ContainerRunning)
	container.SetKnownStatus(ContainerRunning)
	assert.Equal(t, container.GetNextKnownStateProgression(), ContainerStopped)

	resourcesProvisioned := ContainerResourcesProvisioned
	container.steadyState = &resourcesProvisioned
	assert.Equal(t, container.GetNextKnownStateProgression(), ContainerResourcesProvisioned)
	container.SetKnownStatus(ContainerResourcesProvisioned)
	assert.Equal(t, container.GetNextKnownStateProgression(), ContainerStopped)
}
