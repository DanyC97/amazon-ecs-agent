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
	"fmt"
	"strconv"
	"sync"
)

const (
	// DockerContainerMinimumMemoryInBytes is the minimum amount of
	// memory to be allocated to a docker container
	DockerContainerMinimumMemoryInBytes = 4 * 1024 * 1024 // 4MB
)

// ContainerOverrides are overrides applied to the container
type ContainerOverrides struct {
	Command *[]string `json:"command"`
}

// DockerConfig represents additional metadata about a container to run. It's
// remodeled from the `ecsacs` api model file. Eventually it should not exist
// once this remodeling is refactored out.
type DockerConfig struct {
	Config     *string `json:"config"`
	HostConfig *string `json:"hostConfig"`
	Version    *string `json:"version"`
}

// Container is the internal representation of a container in the ECS agent
type Container struct {
	// Name is the name of the container specified in the task definition
	Name string
	// Image is the image name specified in the task definition
	Image string
	// ImageID is the local ID of the image used in the container
	ImageID string

	Command                []string
	CPU                    uint `json:"Cpu"`
	Memory                 uint
	Links                  []string
	VolumesFrom            []VolumeFrom  `json:"volumesFrom"`
	MountPoints            []MountPoint  `json:"mountPoints"`
	Ports                  []PortBinding `json:"portMappings"`
	Essential              bool
	EntryPoint             *[]string
	Environment            map[string]string           `json:"environment"`
	Overrides              ContainerOverrides          `json:"overrides"`
	DockerConfig           DockerConfig                `json:"dockerConfig"`
	RegistryAuthentication *RegistryAuthenticationData `json:"registryAuthentication"`

	// DesiredStatus represents the state where the container should go.
	// Generally the desired status is informed by the ECS backend as a result of
	// either API calls made to ECS or decisions made by the ECS service scheduler, though the
	// agent may also set the DesiredStatus if a different "essential" container in the task
	// exits.  The DesiredStatus is almost always either ContainerRunning or ContainerStopped.
	// Do not access DesiredStatus directly.  Instead, use `GetDesiredStatus` and `SetDesiredStatus`.
	// TODO DesiredStatus should probably be private with appropriately written setter/getter.
	// When this is done, we need to ensure that the UnmarshalJSON is handled properly so that the
	// state storage continues to work.
	DesiredStatus     ContainerStatus `json:"desiredStatus"`
	desiredStatusLock sync.RWMutex

	// KnownStatus represents the state where the container is.  Do not access KnownStatus directly.
	// Instead, use `GetKnownStatus` and `SetKnownStatus`.
	// TODO KnownStatus should probably be private with appropriately written setter/getter.
	// When this is done, we need to ensure that UnmarshalJSON is handled properly so that the
	// state storage continues to work.
	KnownStatus     ContainerStatus
	knownStatusLock sync.RWMutex

	// RunDependencies is a list of containers that must be run before
	// this one is created
	RunDependencies []string
	// 'Internal' containers are ones that are not directly specified by task definitions,
	// but created by the agent
	IsInternal bool

	// AppliedStatus is the status that has been "applied" (e.g., we've called
	// Pull, Create, Start, or Stop) but we don't yet know that the application was successful.
	AppliedStatus ContainerStatus
	// ApplyingError is an error that occured trying to transition the container to its desired state
	// It is propagated to the backend in the form 'Name: ErrorString' as the 'reason' field.
	ApplyingError *DefaultNamedError

	// SentStatus represents the last KnownStatus that was sent to the ECS SubmitContainerStateChange API.
	// TODO SentStatus should probably be private with appropriately written setter/getter.
	// When this is done, we need to ensure that the UnmarshalJSON is handled properly so that the state
	// storage continues to work.
	SentStatus     ContainerStatus
	sentStatusLock sync.RWMutex

	KnownExitCode     *int
	KnownPortBindings []PortBinding

	// ResourceDependecies are TODO
	ResourceDependecies []string
}

// DockerContainer is a mapping between containers-as-docker-knows-them and
// containers-as-we-know-them.
// This is primarily used in DockerState, but lives here such that tasks and
// containers know how to convert themselves into Docker's desired config format
type DockerContainer struct {
	DockerID   string `json:"DockerId"`
	DockerName string // needed for linking

	Container *Container
}

// String returns a human readable string representation of DockerContainer
func (dc *DockerContainer) String() string {
	if dc == nil {
		return "nil"
	}
	return fmt.Sprintf("Id: %s, Name: %s, Container: %s", dc.DockerID, dc.DockerName, dc.Container.String())
}

// Overriden applies the overridden command and returns the resulting
// container object
func (c *Container) Overridden() *Container {
	result := *c

	// We only support Command overrides at the moment
	if result.Overrides.Command != nil {
		result.Command = *c.Overrides.Command
	}

	return &result
}

// KnownTerminal returns true if the container's known status is STOPPED
func (c *Container) KnownTerminal() bool {
	return c.GetKnownStatus().Terminal()
}

// DesiredTerminal returns true if the container's desired status is STOPPED
func (c *Container) DesiredTerminal() bool {
	return c.GetDesiredStatus().Terminal()
}

// GetKnownStatus returns the known status of the container
func (c *Container) GetKnownStatus() ContainerStatus {
	c.knownStatusLock.RLock()
	defer c.knownStatusLock.RUnlock()

	return c.KnownStatus
}

// SetKnownStatus sets the known status of the container
func (c *Container) SetKnownStatus(status ContainerStatus) {
	c.knownStatusLock.Lock()
	defer c.knownStatusLock.Unlock()

	c.KnownStatus = status
}

// GetDesiredStatus gets the desired status of the container
func (c *Container) GetDesiredStatus() ContainerStatus {
	c.desiredStatusLock.RLock()
	defer c.desiredStatusLock.RUnlock()

	return c.DesiredStatus
}

// SetDesiredStatus sets the desired status of the container
func (c *Container) SetDesiredStatus(status ContainerStatus) {
	c.desiredStatusLock.Lock()
	defer c.desiredStatusLock.Unlock()

	c.DesiredStatus = status
}

// GetSentStatus safely returns the SentStatus of the container
func (c *Container) GetSentStatus() ContainerStatus {
	c.sentStatusLock.RLock()
	defer c.sentStatusLock.RUnlock()

	return c.SentStatus
}

// SetSentStatus safely sets the SentStatus of the container
func (c *Container) SetSentStatus(status ContainerStatus) {
	c.sentStatusLock.Lock()
	defer c.sentStatusLock.Unlock()

	c.SentStatus = status
}

// String returns a human readable string representation of this object
func (c *Container) String() string {
	ret := fmt.Sprintf("%s(%s) (%s->%s)", c.Name, c.Image, c.GetKnownStatus().String(), c.GetDesiredStatus().String())
	if c.KnownExitCode != nil {
		ret += " - Exit: " + strconv.Itoa(*c.KnownExitCode)
	}
	return ret
}
