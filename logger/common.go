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
	"time"

	piazza "github.com/venicegeo/dg-pz-gocommon/gocommon"
)

//---------------------------------------------------------------------------

type Stats struct {
	CreatedOn time.Time `json:"createdOn"`

	// this is the number of messages since the service was started,
	// not the total number of messages in the system
	NumMessages int `json:"numMessages"`

	NumMessagesByApplication map[string]int `json:"numMessagesByApplication"`
}

//---------------------------------------------------------------------------

func init() {
	piazza.JsonResponseDataTypes["[]syslog.Message"] = "syslogMessage-list"
	piazza.JsonResponseDataTypes["logger.Stats"] = "logstats"
	piazza.JsonResponseDataTypes["*logger.Stats"] = "logstats"
}

func paginationCreatedOnToTimeStamp(pagination *piazza.JsonPagination) {
	if pagination.SortBy == "createdOn" {
		pagination.SortBy = "timeStamp"
	}
}
