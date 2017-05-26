// Copyright 2016, RadiantBlue Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logger

import (
	"github.com/venicegeo/dg-pz-gocommon/elasticsearch"
	piazza "github.com/venicegeo/dg-pz-gocommon/gocommon"
	pzsyslog "github.com/venicegeo/dg-pz-gocommon/syslog"
)

type Kit struct {
	esi elasticsearch.IIndex

	Service       *Service
	Server        *Server
	LogWriter     pzsyslog.Writer
	AuditWriter   pzsyslog.Writer
	Sys           *piazza.SystemConfig
	GenericServer *piazza.GenericServer
	Url           string
	Async         bool

	done chan error
}

// NewKit starts a logger.Server, using a real or mocked ES backend.
// This is only used for testing.
func NewKit(
	sys *piazza.SystemConfig,
	logWriter pzsyslog.Writer,
	auditWriter pzsyslog.Writer,
	esi elasticsearch.IIndex,
	asyncLogging bool,
) (*Kit, error) {

	var err error

	kit := &Kit{}
	kit.esi = esi
	kit.Service = &Service{}
	kit.Sys = sys
	kit.LogWriter = logWriter
	kit.AuditWriter = auditWriter
	kit.Async = asyncLogging

	err = kit.Service.Init(kit.Sys, kit.LogWriter, kit.AuditWriter, kit.esi, kit.Async)
	if err != nil {
		return nil, err
	}

	kit.Server = &Server{}
	err = kit.Server.Init(kit.Service)
	if err != nil {
		return nil, err
	}

	kit.GenericServer = &piazza.GenericServer{Sys: kit.Sys}
	err = kit.GenericServer.Configure(kit.Server.Routes)
	if err != nil {
		return nil, err
	}

	kit.Url = piazza.DefaultProtocol + "://" + kit.GenericServer.Sys.BindTo

	return kit, nil
}

/////////////

func (kit *Kit) Start() error {
	var err error
	kit.done, err = kit.GenericServer.Start()
	return err
}

func (kit *Kit) Wait() error {
	return <-kit.done
}

func (kit *Kit) Stop() error {
	err := kit.GenericServer.Stop()
	return err
}
