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
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/venicegeo/dg-pz-gocommon/elasticsearch"
	"github.com/venicegeo/dg-pz-gocommon/gocommon"
	pzsyslog "github.com/venicegeo/dg-pz-gocommon/syslog"
)

type Service struct {
	sync.Mutex

	stats  Stats
	origin string
	async  bool

	logWriter   pzsyslog.Writer
	auditWriter pzsyslog.Writer

	esIndex elasticsearch.IIndex
}

func (service *Service) Init(sys *piazza.SystemConfig, logWriter pzsyslog.Writer, auditWriter pzsyslog.Writer, esi elasticsearch.IIndex, asyncLogging bool) error {
	service.stats.CreatedOn = time.Now()
	service.stats.NumMessagesByApplication = map[string]int{}

	service.logWriter = logWriter
	service.auditWriter = auditWriter

	service.esIndex = esi

	service.origin = string(sys.Name)

	service.async = asyncLogging

	return nil
}

func (service *Service) newInternalErrorResponse(err error) *piazza.JsonResponse {
	return &piazza.JsonResponse{
		StatusCode: http.StatusInternalServerError,
		Message:    err.Error(),
		Origin:     service.origin,
	}
}

func (service *Service) newBadRequestResponse(err error) *piazza.JsonResponse {
	return &piazza.JsonResponse{
		StatusCode: http.StatusBadRequest,
		Message:    err.Error(),
		Origin:     service.origin,
	}
}

func (service *Service) incrementStats(application string) {
	service.Lock()
	service.stats.NumMessages++
	service.stats.NumMessagesByApplication[application]++
	service.Unlock()
}

func (service *Service) GetRoot() *piazza.JsonResponse {
	resp := &piazza.JsonResponse{
		StatusCode: 200,
		Data:       "Hi. I'm pz-logger.",
	}

	err := resp.SetType()
	if err != nil {
		return service.newInternalErrorResponse(err)
	}

	return resp
}

func (service *Service) GetStats() *piazza.JsonResponse {
	service.Lock()
	t := service.stats
	service.Unlock()

	resp := &piazza.JsonResponse{
		StatusCode: http.StatusOK,
		Data:       t,
	}

	err := resp.SetType()
	if err != nil {
		return service.newInternalErrorResponse(err)
	}

	return resp
}

func createQueryDslAsString(
	pagination *piazza.JsonPagination,
	params *piazza.HttpQueryParams) (string, error) {

	must := []map[string]interface{}{}

	service, err := params.GetAsString("service", "")
	if err != nil {
		return "", err
	}

	contains, err := params.GetAsString("contains", "")
	if err != nil {
		return "", err
	}

	before, err := params.GetBefore(time.Time{})
	if err != nil {
		return "", err
	}

	after, err := params.GetAfter(time.Time{})
	if err != nil {
		return "", err
	}

	if service != "" {
		must = append(must, map[string]interface{}{
			"match": map[string]interface{}{
				"application": service,
			},
		})
	}
	if contains != "" {
		should := []map[string]interface{}{}
		should = append(should, map[string]interface{}{
			"query": map[string]interface{}{
				"wildcard": map[string]interface{}{
					"hostName": map[string]interface{}{
						"value": "*" + contains + "*",
					},
				},
			},
		})
		should = append(should, map[string]interface{}{
			"query": map[string]interface{}{
				"wildcard": map[string]interface{}{
					"application": map[string]interface{}{
						"value": "*" + contains + "*",
					},
				},
			},
		})
		should = append(should, map[string]interface{}{
			"query": map[string]interface{}{
				"wildcard": map[string]interface{}{
					"process": map[string]interface{}{
						"value": "*" + contains + "*",
					},
				},
			},
		})
		should = append(should, map[string]interface{}{
			"query": map[string]interface{}{
				"wildcard": map[string]interface{}{
					"messageId": map[string]interface{}{
						"value": "*" + contains + "*",
					},
				},
			},
		})
		should = append(should, map[string]interface{}{
			"query": map[string]interface{}{
				"wildcard": map[string]interface{}{
					"message": map[string]interface{}{
						"value": "*" + contains + "*",
					},
				},
			},
		})
		queries := make([]interface{}, len(should))
		for i, s := range should {
			queries[i] = s
		}
		must = append(must, map[string]interface{}{
			"filtered": map[string]interface{}{
				"query": map[string]interface{}{
					"match_all": map[string]interface{}{},
				},
				"filter": map[string]interface{}{
					"bool": map[string]interface{}{
						"should": queries,
					},
				},
			},
		})
	}

	if !after.IsZero() || !before.IsZero() {
		rangeParams := map[string]time.Time{}

		if !after.IsZero() {
			rangeParams["gte"] = after
		}

		if !before.IsZero() {
			rangeParams["lte"] = before
		}

		must = append(must, map[string]interface{}{
			"range": map[string]interface{}{
				"timeStamp": rangeParams,
			},
		})
	}

	if len(must) == 0 {
		return "", nil
	}

	dsl := map[string]interface{}{
		"query": map[string]interface{}{
			"filtered": map[string]interface{}{
				"query": map[string]interface{}{
					"bool": map[string]interface{}{
						"must": must,
					},
				},
			},
		},
		"size": pagination.PerPage,
		"from": pagination.PerPage * pagination.Page,
	}

	dsl["sort"] = map[string]string{
		pagination.SortBy: string(pagination.Order),
	}

	output, err := json.Marshal(dsl)
	if err != nil {
		return "", err
	}

	return string(output), nil
}

func (service *Service) PostSyslog(mNew *pzsyslog.Message) *piazza.JsonResponse {
	err := mNew.Validate()
	if err != nil {
		return service.newBadRequestResponse(err)
	}

	err = service.postSyslog(mNew)
	if err != nil {
		return service.newInternalErrorResponse(err)
	}

	service.incrementStats(mNew.Application)

	resp := &piazza.JsonResponse{
		StatusCode: http.StatusOK,
	}
	return resp
}

func (service *Service) postSyslog(mssg *pzsyslog.Message) error {
	var err error
	isAudit := mssg.AuditData != nil

	if service.logWriter != nil {
		err = service.logWriter.Write(mssg, service.async)
		if err != nil {
			return fmt.Errorf("syslog.Service.postSyslog: %s", err.Error())
		}
	}

	if isAudit && service.auditWriter != nil {
		err = service.auditWriter.Write(mssg, service.async)
		if err != nil {
			return fmt.Errorf("syslog.Service.postSyslog (audit): %s", err.Error())
		}
	}

	return nil
}

func (service *Service) getMessageCommon(params *piazza.HttpQueryParams) (*elasticsearch.SearchResult, *piazza.JsonPagination, *piazza.JsonResponse) {
	pagination, err := piazza.NewJsonPagination(params)
	if err != nil {
		return nil, nil, service.newBadRequestResponse(err)
	}

	paginationCreatedOnToTimeStamp(pagination)

	dsl, err := createQueryDslAsString(pagination, params)
	if err != nil {
		return nil, pagination, service.newBadRequestResponse(err)
	}

	var searchResult *elasticsearch.SearchResult

	if dsl == "" {
		searchResult, err = service.esIndex.FilterByMatchAll(pzsyslog.LoggerType, pagination)
	} else {
		searchResult, err = service.esIndex.SearchByJSON(pzsyslog.LoggerType, dsl)
	}
	if err != nil {
		return nil, pagination, service.newInternalErrorResponse(err)
	}

	return searchResult, pagination, nil
}

func (service *Service) GetSyslog(params *piazza.HttpQueryParams) *piazza.JsonResponse {
	var err error

	format, err := params.GetAsString("format", "json")
	if err != nil {
		return service.newInternalErrorResponse(err)
	}

	searchResult, pagination, jErr := service.getMessageCommon(params)
	if jErr != nil {
		return jErr
	}

	lines, err := extractFromSearchResult(searchResult)
	if err != nil {
		return service.newInternalErrorResponse(err)
	}

	var data interface{} = lines

	if format == "string" {
		t := make([]string, len(lines))
		for i := 0; i < len(lines); i++ {
			t[i] = lines[i].String()
		}
		data = t
	}

	pagination.Count = int(searchResult.TotalHits())
	resp := &piazza.JsonResponse{
		StatusCode: http.StatusOK,
		Data:       data,
		Pagination: pagination,
	}

	err = resp.SetType()
	if err != nil {
		return service.newInternalErrorResponse(err)
	}

	return resp
}

func extractFromSearchResult(searchResult *elasticsearch.SearchResult) ([]pzsyslog.Message, error) {

	var lines = make([]pzsyslog.Message, 0, len(*searchResult.GetHits()))

	if searchResult == nil || searchResult.GetHits() == nil {
		return lines, nil
	}

	for _, hit := range *searchResult.GetHits() {
		if hit.Source == nil {
			log.Printf("null source hit")
			continue
		}

		msg := pzsyslog.Message{}
		err := json.Unmarshal(*hit.Source, &msg)
		if err != nil {
			log.Printf("UNABLE TO PARSE: %s", string(*hit.Source))
			continue
		}

		// just in case
		/*err = msg.Validate()
		if err != nil {
			log.Printf("UNABLE TO VALIDATE: %s", string(*hit.Source))
			continue
		}*/

		lines = append(lines, msg)
	}

	return lines, nil
}

func (service *Service) PostQuery(params *piazza.HttpQueryParams, jsnQuery string) *piazza.JsonResponse {

	pagination, err := piazza.NewJsonPagination(params)
	if err != nil {
		return service.newBadRequestResponse(err)
	}

	searchResult, err := service.esIndex.SearchByJSON(pzsyslog.LoggerType, jsnQuery)
	if err != nil {
		return service.newInternalErrorResponse(err)
	}

	lines, err := extractFromSearchResult(searchResult)
	if err != nil {
		return service.newInternalErrorResponse(err)
	}

	pagination.Count = int(searchResult.TotalHits())
	resp := &piazza.JsonResponse{
		StatusCode: http.StatusOK,
		Data:       lines,
		Pagination: pagination,
	}

	err = resp.SetType()
	if err != nil {
		return service.newInternalErrorResponse(err)
	}

	return resp
}
