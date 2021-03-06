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

// Package acsclient wraps the generated aws-sdk-go client to provide marshalling
// and unmarshalling of data over a websocket connection in the format expected
// by ACS. It allows for bidirectional communication and acts as both a
// client-and-server in terms of requests, but only as a client in terms of
// connecting.
package acsclient

import (
	"errors"

	"github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/credentials"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	wsclient "github.com/aws/amazon-ecs-agent/agent/websocket/client"
)

var log = logger.ForModule("acs client")

// clientServer implements ClientServer for acs.
type clientServer struct {
	wsclient.ClientServerImpl
}

// New returns a client/server to bidirectionally communicate with ACS
// The returned struct should have both 'Connect' and 'Serve' called upon it
// before being used.
func New(url string, region string, credentialProvider credentials.AWSCredentialProvider, acceptInvalidCert bool) wsclient.ClientServer {

	cs := &clientServer{}
	cs.URL = url
	cs.Region = region
	cs.CredentialProvider = credentialProvider
	cs.AcceptInvalidCert = acceptInvalidCert
	cs.ServiceError = &acsError{}
	cs.RequestHandlers = make(map[string]wsclient.RequestHandler)
	cs.TypeMappings = &typeMappings{}
	cs.TypeDecoder = &decoder{}

	return cs
}

// Serve begins serving requests using previously registered handlers (see
// AddRequestHandler). All request handlers should be added prior to making this
// call as unhandled requests will be discarded.
func (cs *clientServer) Serve() error {
	log.Debug("Starting websocket poll loop")
	if cs.Conn == nil {
		return errors.New("nil connection")
	}

	return cs.ConsumeMessages()
}

// Close closes the underlying connection
func (cs *clientServer) Close() error {
	return cs.Conn.Close()
}
