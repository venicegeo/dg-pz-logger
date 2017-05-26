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
	"net/http"

	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/venicegeo/dg-pz-gocommon/gocommon"
	syslogger "github.com/venicegeo/dg-pz-gocommon/syslog"
)

type Server struct {
	service *Service
	Routes  []piazza.RouteData
}

const Version = "1.0.0"

func (server *Server) Init(service *Service) error {
	server.service = service

	server.Routes = []piazza.RouteData{
		{Verb: "GET", Path: "/", Handler: server.handleGetRoot},
		{Verb: "GET", Path: "/version", Handler: server.handleGetVersion},
		{Verb: "GET", Path: "/admin/stats", Handler: server.handleGetStats},

		{Verb: "GET", Path: "/syslog", Handler: server.handleGetSyslog},
		{Verb: "POST", Path: "/syslog", Handler: server.handlePostSyslog},

		{Verb: "POST", Path: "/query", Handler: server.handlePostQuery},
	}

	return nil
}

func (server *Server) handleGetRoot(c *gin.Context) {
	resp := server.service.GetRoot()
	piazza.GinReturnJson(c, resp)
}

func (server *Server) handleGetVersion(c *gin.Context) {
	version := piazza.Version{Version: Version}
	resp := &piazza.JsonResponse{StatusCode: http.StatusOK, Data: version}
	piazza.GinReturnJson(c, resp)
}

func (server *Server) handleGetStats(c *gin.Context) {
	resp := server.service.GetStats()
	piazza.GinReturnJson(c, resp)
}

func (server *Server) handleGetSyslog(c *gin.Context) {
	params := piazza.NewQueryParams(c.Request)
	resp := server.service.GetSyslog(params)

	piazza.GinReturnJson(c, resp)
}

func (server *Server) handlePostSyslog(c *gin.Context) {
	sysM := syslogger.NewMessage()

	err := c.BindJSON(&sysM)
	if err != nil {
		resp := &piazza.JsonResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
		piazza.GinReturnJson(c, resp)
		return
	}
	resp := server.service.PostSyslog(sysM)
	piazza.GinReturnJson(c, resp)
}

func (server *Server) handlePostQuery(c *gin.Context) {
	params := piazza.NewQueryParams(c.Request)

	// We have been given a string (containing JSON) and we want to
	// pass that to the service handler. There doesn't appear to be
	// a BindString function, so we will convert that to an dumb object
	// in the normal way, and then decode that object back out to a
	// JSON string.
	var obj interface{}

	err := c.Bind(&obj)
	if err != nil {
		resp := &piazza.JsonResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
		piazza.GinReturnJson(c, resp)
		return
	}

	byts, err := json.Marshal(obj)
	if err != nil {
		resp := &piazza.JsonResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "handlePostQuery: bad string format",
		}
		piazza.GinReturnJson(c, resp)
		return
	}

	resp := server.service.PostQuery(params, string(byts))
	piazza.GinReturnJson(c, resp)
}
