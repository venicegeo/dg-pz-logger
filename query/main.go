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

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/venicegeo/dg-pz-gocommon/gocommon"
	pzsyslog "github.com/venicegeo/dg-pz-gocommon/syslog"
)

const LoggerUrl = "http://pz-logger.int.geointservices.io"

func main() {

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

	array, err := readMessages(jsn)
	errcheck(err)

	count := len(array)

	for _, mssg := range array {

		date := mssg.TimeStamp.String()
		app := mssg.Application
		sev := SeverityString(mssg.Severity)
		text := mssg.Message
		_ = date + app + sev + text

		audit := "-"
		if mssg.AuditData != nil {
			audit = mssg.AuditData.String()
		}
		metric := "-"
		if mssg.MetricData != nil {
			metric = mssg.MetricData.String()
		}
		source := "-"
		if mssg.SourceData != nil {
			source = mssg.SourceData.String()
		}
		_ = audit + metric + source

		// fac := mssg.Facility
		// ver := mssg.Version
		// host := mssg.HostName
		// pid := mssg.Process
		// id := mssg.MessageID

		text = trimText(text)

		fmt.Printf("%s\t%s\t%s\n", app, sev, text)
	}

	tstart := time.Time(array[0].TimeStamp)
	tend := time.Time(array[count-1].TimeStamp)
	deltaS := tend.Sub(tstart).Seconds()
	deltaM := tend.Sub(tstart).Minutes()
	deltaH := tend.Sub(tstart).Hours()
	perS := float64(count) / deltaS
	perM := float64(count) / deltaM
	if perM < 1.0 {
		log.Printf("Read %d messages covering %.0f seconds: %.0f per second", count, deltaS, perS)
	} else {
		if deltaM < 60.0 {
			log.Printf("Read %d messages covering %.0f minutes: %.0f per minute", count, deltaM, perM)

		} else {
			log.Printf("Read %d messages covering %.1f hours: %.0f per minute", count, deltaH, perM)
		}
	}
}

func errcheck(err error) {
	if err == nil {
		return
	}
	log.Fatalf("ERROR: %s", err.Error())
}

var severityStrings map[pzsyslog.Severity]string

func init() {
	severityStrings = map[pzsyslog.Severity]string{
		pzsyslog.Emergency:     "Emergency",
		pzsyslog.Alert:         "Alert",
		pzsyslog.Fatal:         "Fatal",
		pzsyslog.Error:         "Error",
		pzsyslog.Warning:       "Warning",
		pzsyslog.Notice:        "Notice",
		pzsyslog.Informational: "Informational",
		pzsyslog.Debug:         "Debug",
	}
}

func SeverityString(severity pzsyslog.Severity) string {
	return severityStrings[severity]
}

func trimText(text string) string {

	// strip leading/trailing whitespace
	text = strings.Trim(text, " \t\n")

	// end the string if an embedded newline
	end := strings.Index(text, "\n")
	if end > 0 {
		text = text[:end]
	}

	// replace uuids
	re := regexp.MustCompile("[-0123456789abcdef]{36}")
	text = re.ReplaceAllString(text, "###")

	// limit len to at most 50 chars
	mini := func(i, j int) int {
		fi := float64(i)
		fj := float64(j)
		fm := math.Min(fi, fj)
		return int(fm)
	}
	end = mini(len(text), 50)
	text = text[:end]

	// strip leading/trailing whitespace
	text = strings.Trim(text, " \t\n")

	return text
}

func readMessages(jsn string) ([]pzsyslog.Message, error) {
	h := &piazza.Http{BaseUrl: LoggerUrl}

	resp := h.PzPost("/query", jsn)

	if resp.IsError() {
		return nil, resp.ToError()
	}

	bytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, err
	}

	array := []pzsyslog.Message{}
	err = json.Unmarshal(bytes, &array)
	if err != nil {
		return nil, err
	}

	//fmt.Printf("BYTES %s\n", string(bytes))
	//fmt.Printf("ARY %#v\n", array)

	return array, nil
}
