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

package systest

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"encoding/json"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/venicegeo/dg-pz-gocommon/gocommon"
	pzsyslog "github.com/venicegeo/dg-pz-gocommon/syslog"
)

func sleep(n int) {
	time.Sleep(time.Second * time.Duration(n))
}

type LoggerTester struct {
	suite.Suite

	logWriter   pzsyslog.Writer
	httpWriter  *pzsyslog.HttpWriter // just a typed copy of logWriter
	auditWriter pzsyslog.Writer

	logger    *pzsyslog.Logger
	apiKey    string
	apiHost   string
	loggerUrl string

	mssgHostName    string
	mssgApplication string
	mssgProcess     string
	mmsgSeverity    pzsyslog.Severity
}

func (suite *LoggerTester) setupFixture() {
	t := suite.T()
	assert := assert.New(t)

	var err error

	suite.apiHost, err = piazza.GetApiServer()
	if err != nil {
		assert.FailNow(err.Error())
	}

	// note that we are NOT using the gateway
	suite.loggerUrl, err = piazza.GetPiazzaServiceUrl(piazza.PzLogger)
	assert.NoError(err)

	suite.apiKey, err = piazza.GetApiKey(suite.apiHost)
	assert.NoError(err)

	suite.httpWriter, err = pzsyslog.NewHttpWriter(suite.loggerUrl, suite.apiKey)
	suite.logWriter = suite.httpWriter
	assert.NoError(err)

	suite.auditWriter, err = pzsyslog.NewHttpWriter(suite.loggerUrl, suite.apiKey)

	suite.mssgHostName, err = piazza.GetExternalIP()
	assert.NoError(err)
	suite.mssgApplication = "pz-logger/systest"
	suite.mssgProcess = strconv.Itoa(os.Getpid())
	suite.mmsgSeverity = pzsyslog.Informational

	suite.logger = pzsyslog.NewLogger(suite.logWriter, suite.auditWriter, suite.mssgApplication)
}

func (suite *LoggerTester) teardownFixture() {
	t := suite.T()
	assert := assert.New(t)

	err := suite.logWriter.Close()
	assert.NoError(err)
}

func TestRunSuite(t *testing.T) {
	s := &LoggerTester{}
	suite.Run(t, s)
}

func (suite *LoggerTester) verifyMessage(expected string) {
	format := &piazza.JsonPagination{
		PerPage: 500, // has to be this high, in case logger is under high load
		Page:    0,
		SortBy:  "timeStamp",
		Order:   piazza.SortOrderDescending,
	}
	params := &piazza.HttpQueryParams{}
	suite.verifyMessageF(format, params, expected)
}

func (suite *LoggerTester) verifyMessageF(
	format *piazza.JsonPagination,
	params *piazza.HttpQueryParams,
	expected string,
) {
	t := suite.T()
	assert := assert.New(t)

	ms, _, err := suite.httpWriter.GetMessages(format, params)
	assert.NoError(err)
	assert.Len(ms, format.PerPage)

	ok := false
	for _, m := range ms {
		if m.Message == expected {
			ok = true
			break
		}
	}
	assert.True(ok)
}

func (suite *LoggerTester) getVersion() (*piazza.Version, error) {
	h := &piazza.Http{BaseUrl: suite.loggerUrl}
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
	h := &piazza.Http{BaseUrl: suite.loggerUrl}

	jresp := h.PzGet("/admin/stats")
	if jresp.IsError() {
		return jresp.ToError()
	}

	return jresp.ExtractData(output)
}

func (suite *LoggerTester) makeMessage(text string) *pzsyslog.Message {
	t := suite.T()
	assert := assert.New(t)

	var err error

	m := pzsyslog.NewMessage()
	m.Message = text
	m.HostName, err = piazza.GetExternalIP()
	assert.NoError(err)
	m.Application = "pzlogger-systest"
	m.Process = strconv.Itoa(os.Getpid())
	m.Severity = pzsyslog.Informational

	return m
}

func (suite *LoggerTester) Test00Version() {
	t := suite.T()
	assert := assert.New(t)

	suite.setupFixture()
	defer suite.teardownFixture()

	version, err := suite.getVersion()
	assert.NoError(err)
	assert.EqualValues("1.0.0", version.Version)
}

func (suite *LoggerTester) Test01RawGet() {
	t := suite.T()
	assert := assert.New(t)

	suite.setupFixture()
	defer suite.teardownFixture()
	resp, err := http.Get(suite.loggerUrl + "/syslog")
	assert.NoError(err)
	if resp.ContentLength <= 0 {
		log.Printf("content-length is <= 0")
		panic(99)
	}
	assert.True(resp.ContentLength > 0)

	raw := make([]byte, resp.ContentLength)
	_, err = io.ReadFull(resp.Body, raw)
	defer func() {
		err = resp.Body.Close()
		assert.NoError(err)
	}()
	if err != nil && err != io.EOF {
		assert.NoError(err)
	}

	assert.Equal(200, resp.StatusCode)
}

func (suite *LoggerTester) Test02RawPost() {
	t := suite.T()
	assert := assert.New(t)

	suite.setupFixture()
	defer suite.teardownFixture()

	mssg := suite.makeMessage("Test02")

	jsn, err := json.Marshal(mssg)
	reader := bytes.NewReader(jsn)

	resp, err := http.Post(suite.loggerUrl+"/syslog",
		piazza.ContentTypeJSON, reader)
	assert.NoError(err)

	raw := make([]byte, resp.ContentLength)
	_, err = io.ReadFull(resp.Body, raw)
	defer func() {
		err = resp.Body.Close()
		assert.NoError(err)
	}()
	if err != nil && err != io.EOF {
		assert.NoError(err)
	}

	assert.Equal(200, resp.StatusCode)
}

func (suite *LoggerTester) Test03Get() {
	t := suite.T()
	assert := assert.New(t)

	suite.setupFixture()
	defer suite.teardownFixture()

	format := &piazza.JsonPagination{
		PerPage: 12,
		Page:    0,
		Order:   piazza.SortOrderDescending,
		SortBy:  "hostName",
	}
	ms, _, err := suite.httpWriter.GetMessages(format, nil)
	assert.NoError(err)
	assert.Len(ms, format.PerPage)

	assert.False(time.Time(ms[0].TimeStamp).IsZero())
}

func (suite *LoggerTester) Test04Post() {
	t := suite.T()
	assert := assert.New(t)

	suite.setupFixture()
	defer suite.teardownFixture()

	var err error

	nowz := piazza.NewTimeStamp()
	keyz := "KEYZ KEYZ KEYZ " + nowz.String()

	mssgz := suite.makeMessage(keyz)
	mssgz.TimeStamp = nowz

	err = suite.httpWriter.Write(mssgz, false)
	assert.NoError(err)

	// allow ES to catch up
	sleep(2)

	suite.verifyMessage(keyz)
}

func (suite *LoggerTester) Test05Logger() {
	t := suite.T()
	assert := assert.New(t)

	suite.setupFixture()
	defer suite.teardownFixture()

	uniq := "Test05/" + time.Now().String()

	err := suite.logger.Info(uniq)
	assert.NoError(err)

	sleep(2)

	suite.verifyMessage(uniq)
}

func (suite *LoggerTester) Test06Admin() {
	t := suite.T()
	assert := assert.New(t)

	suite.setupFixture()
	defer suite.teardownFixture()

	output := map[string]interface{}{}
	err := suite.getStats(&output)
	assert.NoError(err)
	assert.NotZero(output["numMessages"])
}

func (suite *LoggerTester) Test07Pagination() {
	t := suite.T()
	assert := assert.New(t)

	suite.setupFixture()
	defer suite.teardownFixture()

	format := &piazza.JsonPagination{
		PerPage: 10,
		Page:    0,
		SortBy:  "timeStamp",
		Order:   piazza.SortOrderAscending,
	}
	params := &piazza.HttpQueryParams{}

	// check per-page
	{
		format.PerPage = 17
		ms, _, err := suite.httpWriter.GetMessages(format, params)
		assert.NoError(err)
		assert.Len(ms, 17)
	}

	// check sort order
	{
		format.PerPage = 10
		format.Order = piazza.SortOrderAscending
		ms, _, err := suite.httpWriter.GetMessages(format, params)
		assert.NoError(err)
		last := len(ms) - 1
		assert.True(last <= 9)

		// we can't check "strictly before", because two timeStamps might be the same
		t0 := time.Time(ms[0].TimeStamp)
		tlast := time.Time(ms[last].TimeStamp)
		isBefore := t0.Before(tlast)
		isEqual := t0.Equal(tlast)
		assert.True(isBefore || isEqual)

		format.Order = piazza.SortOrderDescending
		ms, _, err = suite.httpWriter.GetMessages(format, params)
		assert.NoError(err)
		last = len(ms) - 1
		assert.True(last <= 9)

		t0 = time.Time(ms[0].TimeStamp)
		tlast = time.Time(ms[last].TimeStamp)
		isAfter := t0.After(tlast)
		isEqual = t0.Equal(tlast)
		assert.True(isAfter || isEqual)
	}

	// check sort-by
	{
		format.Order = piazza.SortOrderAscending
		format.SortBy = "severity"
		format.PerPage = 100
		format.Page = 0
		ms, _, err := suite.httpWriter.GetMessages(format, params)
		assert.NoError(err)

		last := len(ms) - 1
		for i := 0; i < last; i++ {
			a, b := ms[i].Severity, ms[i+1].Severity
			isSameOrBefore := (a <= b)
			assert.True(isSameOrBefore)
		}
	}
}

func (suite *LoggerTester) Test08Params() {
	t := suite.T()
	assert := assert.New(t)

	suite.setupFixture()
	defer suite.teardownFixture()

	uniq := "Test08/" + strconv.Itoa(time.Now().Nanosecond())

	delta := time.Duration(10 * time.Second)
	tstart := time.Now().Add(-delta).UTC()

	err := suite.logger.Information(uniq)
	assert.NoError(err)

	tend := time.Now().Add(delta).UTC()

	sleep(1)

	format := &piazza.JsonPagination{
		PerPage: 256,
		Page:    0,
		Order:   piazza.SortOrderDescending,
		SortBy:  "timeStamp",
	}

	// test date range params
	{

		params := &piazza.HttpQueryParams{}
		params.AddTime("after", tstart)
		params.AddTime("before", tend)

		msgs, cnt, err := suite.httpWriter.GetMessages(format, params)
		assert.NoError(err)

		assert.True(cnt >= 1)
		assert.True(len(msgs) >= 1)
	}

	// test service param
	{
		params := &piazza.HttpQueryParams{}
		params.AddString("service", suite.mssgApplication)

		msgs, _, err := suite.httpWriter.GetMessages(format, params)
		assert.NoError(err)

		assert.Len(msgs, 1)
	}

	// test contains param
	{
		params := &piazza.HttpQueryParams{}
		params.AddString("contains", suite.mssgHostName)

		msgs, _, err := suite.httpWriter.GetMessages(format, params)
		assert.NoError(err)

		assert.True(len(msgs) >= 1)
	}
}

func (suite *LoggerTester) Test09Query() {
	t := suite.T()
	assert := assert.New(t)

	suite.setupFixture()
	defer suite.teardownFixture()

	jsn := `
{
    "query": {
        "match_all": {}
    },
	"size": 5,
	"from": 0,
	"sort": {
		"timeStamp": "asc"
	}
}`

	h := &piazza.Http{BaseUrl: suite.loggerUrl}
	resp := h.PzPost("/query", jsn)

	assert.False(resp.IsError())
	assert.Nil(resp.ToError())

	data := []pzsyslog.Message{}
	err := resp.ExtractData(&data)
	assert.NoError(err)

	assert.Len(data, 5)
}
