// +build linux

// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package statemanager

import (
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/async"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	log "github.com/cihub/seelog"
	"github.com/vishvananda/netlink"
)

const (
	maxUnmanagedDevicesInCache = 10
	unmanagedDevicesCacheTTL   = time.Hour
)

// StateManager defines the method to manage the state of eni
type StateManager interface {
	Init(state []netlink.Link)
	Reconcile(currentState map[string]string)
	HandleENIEvent(mac string)
}

// stateManager handles the state change of eni
type stateManager struct {
	agentState       dockerstate.TaskEngineState
	eniChangeEvent   chan statechange.Event
	unamangedDevices async.Cache
}

// New returns a new StateManager
func New(state dockerstate.TaskEngineState, event chan statechange.Event) StateManager {
	return &stateManager{
		agentState:       state,
		eniChangeEvent:   event,
		unamangedDevices: async.NewLRUCache(maxUnmanagedDevicesInCache, unmanagedDevicesCacheTTL),
	}
}

// Init populates the initial state of the map
func (statemanager *stateManager) Init(state []netlink.Link) {
	for _, link := range state {
		macAddress := link.Attrs().HardwareAddr.String()
		statemanager.HandleENIEvent(macAddress)
	}
}

// ENIStateChangeShouldBeSent checks whether this eni is managed by ecs
// and if its status should be sent to backend
func (statemanager *stateManager) ENIStateChangeShouldBeSent(macAddress string) (*api.ENIAttachment, bool) {
	if macAddress == "" {
		log.Warn("ENI state manager: device with empty mac address")
		return nil, false
	}
	// check if this is an eni required by a task
	eni, ok := statemanager.agentState.ENIByMac(macAddress)
	if !ok {
		if _, found := statemanager.unamangedDevices.Get(macAddress); !found {
			log.Infof("ENI state manager: device not managed by ecs: %s", macAddress)
			statemanager.unamangedDevices.Set(macAddress, struct{}{})
		}
		return nil, false
	}

	if eni.IsSent() {
		log.Infof("ENI state manager: eni attach status has already sent: %s", macAddress)
		return eni, false
	}

	return eni, true
}

// HandleENIEvent handles the eni event from udev or reconcil phase
func (statemanager *stateManager) HandleENIEvent(mac string) {
	eni, ok := statemanager.ENIStateChangeShouldBeSent(mac)
	if ok {
		eni.Status = api.ENIAttached
		statemanager.emitENIAttachmentEvent(api.TaskStateChange{
			TaskArn:     eni.TaskArn,
			Attachments: eni,
		})
	}
}

// emitENIAttachmentEvent send the eni statechange(attach) to event handler
func (statemanager *stateManager) emitENIAttachmentEvent(event statechange.Event) {
	log.Infof("ENI state manager: sending eni state change to event handler: %v", event)
	statemanager.eniChangeEvent <- event
}

// Reconcile performs a reconciliation of the eni on the instance
func (statemanager *stateManager) Reconcile(currentState map[string]string) {
	// Add new interfaces next
	for mac, _ := range currentState {
		statemanager.HandleENIEvent(mac)
	}
}
