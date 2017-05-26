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
	"fmt"
	"log"
	"net/http"
	"testing"
	"time"

	"os"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/venicegeo/dg-pz-gocommon/elasticsearch"
	piazza "github.com/venicegeo/dg-pz-gocommon/gocommon"
	pzsyslog "github.com/venicegeo/dg-pz-gocommon/syslog"
)

//---------------------------------------------------------------------

type LoggerTester struct {
	suite.Suite

	kit *Kit

	//	logWriter pzsyslog.Writer
	logReader *pzsyslog.LocalReaderWriter
	logger    *pzsyslog.Logger
}

func (suite *LoggerTester) setupFixture() {
	t := suite.T()
	assert := assert.New(t)

	// may need to update these over time
	{
		err := os.Setenv("AUDIT_TYPE", "AuditData")
		assert.NoError(err)
		err = os.Setenv("LOGGER_TYPE", "LoggerData")
		assert.NoError(err)
		err = os.Setenv("LOGGER_INDEX", "pzlogger4")
		assert.NoError(err)
	}

	// set up the logger server
	{
		required := []piazza.ServiceName{}
		sys, err := piazza.NewSystemConfig(piazza.PzLogger, required)
		assert.NoError(err)

		idx, logESWriter, err := setupES(sys)
		assert.NoError(err)

		rwLogReader := &pzsyslog.LocalReaderWriter{}
		auditWriter := rwLogReader

		rwLogWriter := &pzsyslog.LocalReaderWriter{}
		logWriter := pzsyslog.NewMultiWriter([]pzsyslog.Writer{logESWriter, rwLogWriter})

		suite.kit, err = NewKit(sys, logWriter, auditWriter, idx, false)
		assert.NoError(err)

		err = suite.kit.Start()
		assert.NoError(err)

		suite.logReader = rwLogWriter // backdoor, for testing
	}

	// set up the client support
	{
		writer, err := pzsyslog.NewHttpWriter(suite.kit.Url, "")
		assert.NoError(err)
		suite.logger = pzsyslog.NewLogger(writer, writer, "pz-logger/unittest")
	}
}

func setupES(sys *piazza.SystemConfig) (elasticsearch.IIndex, pzsyslog.Writer, error) {
	loggerIndex, err := pzsyslog.GetRequiredEnvVars()
	if err != nil {
		log.Fatal(err)
	}

	idx := elasticsearch.NewMockIndex(loggerIndex)

	logESWriter := pzsyslog.NewElasticWriter(idx, pzsyslog.LoggerType)
	if _, err = logESWriter.CreateIndex(); err != nil {
		return idx, nil, err
	}
	if _, err = logESWriter.CreateType("{}"); err != nil {
		return idx, nil, err
	}

	return idx, logESWriter, nil
}

func (suite *LoggerTester) teardownFixture() {
	t := suite.T()
	assert := assert.New(t)

	err := suite.kit.Stop()
	assert.NoError(err)
}

func TestRunSuite(t *testing.T) {
	s := &LoggerTester{}
	suite.Run(t, s)
}

//---------------------------------------------------------------------

func (suite *LoggerTester) getVersion() (*piazza.Version, error) {
	h := &piazza.Http{BaseUrl: suite.kit.Url}

	jresp := h.PzGet("/version")
	if jresp.IsError() {
		return nil, jresp.ToError()
	}

	var version piazza.Version
	err := jresp.ExtractData(&version)
	if err != nil {
		return nil, err
	}

	return &version, nil
}

func (suite *LoggerTester) getStats(output interface{}) error {
	h := &piazza.Http{BaseUrl: suite.kit.Url}

	jresp := h.PzGet("/admin/stats")
	if jresp.IsError() {
		return jresp.ToError()
	}

	return jresp.ExtractData(output)
}

func sleep() {
	time.Sleep(1 * time.Second)
}

func (suite *LoggerTester) getLastMessage() string {
	t := suite.T()
	assert := assert.New(t)

	ms, err := suite.logReader.Read(1)
	assert.NoError(err)
	assert.True(len(ms) > 0)

	return ms[len(ms)-1].String()
}

//---------------------------------------------------------------------

func (suite *LoggerTester) Test00Time() {
	t := suite.T()
	assert := assert.New(t)

	a := "2006-01-02T15:04:05+07:00"
	b, err := time.Parse(time.RFC3339, a)
	assert.NoError(err)
	c := b.Format(time.RFC3339)
	assert.EqualValues(a, c)
}

func (suite *LoggerTester) Test01Version() {
	t := suite.T()
	assert := assert.New(t)

	suite.setupFixture()
	defer suite.teardownFixture()

	version, err := suite.getVersion()
	assert.NoError(err)
	assert.EqualValues("1.0.0", version.Version)
	_, _, _, err = piazza.HTTP(piazza.GET, fmt.Sprintf("http://localhost:%s/version", piazza.LocalPortNumbers[piazza.PzLogger]), piazza.NewHeaderBuilder().AddJsonContentType().GetHeader(), nil)
	assert.NoError(err)
}

func (suite *LoggerTester) Test02Admin() {
	t := suite.T()
	assert := assert.New(t)

	suite.setupFixture()
	defer suite.teardownFixture()

	output := &map[string]interface{}{}
	err := suite.getStats(output)
	assert.NoError(err, "GetFromAdminStats")
	assert.NotNil(output)

	_, _, _, err = piazza.HTTP(piazza.GET, fmt.Sprintf("http://localhost:%s/admin/stats", piazza.LocalPortNumbers[piazza.PzLogger]), piazza.NewHeaderBuilder().AddJsonContentType().GetHeader(), nil)
	assert.NoError(err)

}

func (suite *LoggerTester) Test03Pagination() {

	t := suite.T()
	assert := assert.New(t)

	suite.setupFixture()
	defer suite.teardownFixture()

	var err error

	err = suite.logger.Debug("d")
	assert.NoError(err)
	err = suite.logger.Info("i")
	assert.NoError(err)
	err = suite.logger.Warning("w")
	assert.NoError(err)
	err = suite.logger.Error("e")
	assert.NoError(err)
	err = suite.logger.Fatal("f")
	assert.NoError(err)

	sleep()

	ms, err := suite.logReader.Read(1)
	assert.NoError(err)
	_, _, _, err = piazza.HTTP(piazza.GET, fmt.Sprintf("http://localhost:%s/syslog?page=0", piazza.LocalPortNumbers[piazza.PzLogger]), piazza.NewHeaderBuilder().AddJsonContentType().GetHeader(), nil)
	assert.NoError(err)

	assert.Len(ms, 1)
	assert.EqualValues(pzsyslog.Fatal, ms[0].Severity)

	ms, err = suite.logReader.Read(5)
	assert.NoError(err)
	assert.Len(ms, 5)
	assert.EqualValues(pzsyslog.Debug, ms[0].Severity)
	assert.EqualValues(pzsyslog.Fatal, ms[4].Severity)

	ms, err = suite.logReader.Read(8)
	assert.NoError(err)
	assert.Len(ms, 5)

	assert.EqualValues(pzsyslog.Error, ms[3].Severity)
	assert.EqualValues(pzsyslog.Fatal, ms[4].Severity)
}

// this test uses an ES query that is not supported under mocking
/*
func (suite *LoggerTester) Test04OtherParams() {

	t := suite.T()
	assert := assert.New(t)

	suite.setupFixture()
	defer suite.teardownFixture()

	//client.SetService("myservice", "1.2.3.4")

	sysloggerD := syslog.NewLogger(&syslog.MessageWriter{}, "Dispatcher")
	sysloggerJ := syslog.NewLogger(&syslog.MessageWriter{}, "JobManager")
	sysloggerU := syslog.NewLogger(&syslog.MessageWriter{}, "pz-uuidgen")

	sysloggerD.Info("Received Message to Relay on topic Request-Job with key f3b63085-b482-4ae8-8297-3c7d1fcfff7d")
	sysloggerJ.Info("Processed Update Status for Job 6d0ea538-4382-4ea5-9669-56519b8c8f58 with Status Success")
	sysloggerU.Info("generated 1: 09d4ec60-ea61-4066-8697-5568a47f84bf")
	sysloggerJ.Info("Handling Job with Topic Create-Job for Job ID 09d4ec60-ea61-4066-8697-5568a47f84b")
	sysloggerJ.Info("Handling Job with Topic Update-Job for Job ID be4ce034-1187-4a4f-95a9-a9c31826248b")

	sleep()

	format := piazza.JsonPagination{
		PerPage: 100,
		Page:    0,
		Order:   "",
		SortBy:  "",
	}

	params := piazza.HttpQueryParams{}
	params.AddString("service", "JobManager")
	params.AddString("contains", "Success")

	msgs, count, err := suite.client.GetMessages(&format, &params)
	assert.NoError(err)
	assert.Len(msgs, 1)
	assert.Equal(5, count)

}
*/

func (suite *LoggerTester) Test05ConstructDsl() {
	t := suite.T()
	assert := assert.New(t)

	format := &piazza.JsonPagination{
		PerPage: 100,
		Page:    0,
		Order:   piazza.SortOrderDescending,
		SortBy:  "timeStamp",
	}

	startS := "2016-07-26T01:00:00.000Z"
	endS := "2016-07-26T02:00:00.000Z"

	startT, err := time.Parse(time.RFC3339, startS)
	assert.NoError(err)
	endT, err := time.Parse(time.RFC3339, endS)
	assert.NoError(err)

	params := &piazza.HttpQueryParams{}
	params.AddTime("before", startT)
	params.AddTime("after", endT)
	params.AddString("service", "myservice")
	params.AddString("contains", "mycontains")

	actual, err := createQueryDslAsString(format, params)
	assert.NoError(err)
	assert.NotEmpty(actual)

	expected := `
	{
		"from":0,
		"query": {
			"filtered":{
				"query":{
					"bool":{
						"must":
						[
							{
								"match":{"application":"myservice"}
							},
							{
								"filtered":{
									"filter":{
										"bool":{
											"should":[
												{"query":{"wildcard":{"hostName":{"value":"*mycontains*"}}}},
												{"query":{"wildcard":{"application":{"value":"*mycontains*"}}}},
												{"query":{"wildcard":{"process":{"value":"*mycontains*"}}}},
												{"query":{"wildcard":{"messageId":{"value":"*mycontains*"}}}},
												{"query":{"wildcard":{"message":{"value":"*mycontains*"}}}}
											]
										}
									},
									"query":{"match_all":{}}
								}
							},
							{
								"range":
								{
									"timeStamp":{
										"lte":"2016-07-26T01:00:00Z", "gte":"2016-07-26T02:00:00Z"
									}
								}
							}
						]
					}
				}
			}
		},
		"size":100,
		"sort":{"timeStamp":"desc"}
	}`
	assert.JSONEq(expected, actual)
}

func (suite *LoggerTester) Test06Server() {
	t := suite.T()
	assert := assert.New(t)

	service := Service{origin: "yow"}

	resp := service.newInternalErrorResponse(fmt.Errorf("foo"))
	assert.Equal(http.StatusInternalServerError, resp.StatusCode)
	assert.Equal("foo", resp.Message)
	assert.Equal("yow", resp.Origin)

	resp = service.newBadRequestResponse(fmt.Errorf("bar"))
	assert.Equal(http.StatusBadRequest, resp.StatusCode)
	assert.Equal("bar", resp.Message)
	assert.Equal("yow", resp.Origin)
}

func (suite *LoggerTester) Test07Syslog() {
	t := suite.T()
	assert := assert.New(t)

	suite.setupFixture()
	defer suite.teardownFixture()

	var err error

	{
		s := "The quick brown fox"
		err = suite.logger.Warning(s)
		assert.NoError(err)
		sleep()
		actual := suite.getLastMessage()
		assert.Contains(actual, s)
		pri := fmt.Sprintf("<%d>%d",
			8*pzsyslog.DefaultFacility+pzsyslog.Warning.Value(), pzsyslog.DefaultVersion)
		assert.Contains(actual, pri)
	}

	{
		s := "The lazy dog"
		err := suite.logger.Error(s)
		assert.NoError(err)
		sleep()
		actual := suite.getLastMessage()
		assert.Contains(actual, s)
		pri := fmt.Sprintf("<%d>%d",
			8*pzsyslog.DefaultFacility+pzsyslog.Error.Value(), pzsyslog.DefaultVersion)
		assert.Contains(actual, pri)
	}

	{
		output := map[string]interface{}{}
		err := suite.getStats(&output)
		assert.NoError(err)
		assert.EqualValues(2, output["numMessages"])
	}

	{
		err := suite.logger.Audit("123", "login!", "456", "789")
		assert.NoError(err)
		sleep()

		actual := suite.getLastMessage()
		assert.Contains(actual, "login")
		pri := fmt.Sprintf("<%d>%d",
			8*pzsyslog.DefaultFacility+pzsyslog.Notice.Value(), pzsyslog.DefaultVersion)
		assert.Contains(actual, pri)
	}
}

func (suite *LoggerTester) Test08PostQuery() {
	t := suite.T()
	assert := assert.New(t)

	suite.setupFixture()
	defer suite.teardownFixture()

	h := &piazza.Http{BaseUrl: suite.kit.Url}

	jsn := `
{
    "query": {
        "match_all": {}
    },
	"size": 10000,
	"from": 0,
	"sort": {
		"timeStamp": "asc"
	}
}`

	resp := h.PzPost("/query", jsn)
	assert.True(resp.IsError())
	assert.Error(resp.ToError())
	assert.Contains(resp.ToError().Error(), "not supported under mocking")
}
