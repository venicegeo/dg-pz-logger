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
	"bytes"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"reflect"
	"regexp"

	"github.com/venicegeo/dg-pz-gocommon/elasticsearch"
	"github.com/venicegeo/dg-pz-gocommon/gocommon"
	pzsyslog "github.com/venicegeo/dg-pz-gocommon/syslog"
	pzlogger "github.com/venicegeo/dg-pz-logger/logger"
)

func main() {

	required := []piazza.ServiceName{piazza.PzElasticSearch}

	sys, err := piazza.NewSystemConfig(piazza.PzLogger, required)
	if err != nil {
		log.Fatal(err)
	}

	idx, logESWriter, auditWriter, err := setupES(sys)
	if err != nil {
		log.Fatal(err)
	}

	kit, err := pzlogger.NewKit(sys, logESWriter, auditWriter, idx, true)
	if err != nil {
		log.Fatal(err)
	}

	err = kit.Start()
	if err != nil {
		log.Fatal(err)
	}

	err = kit.Wait()
	if err != nil {
		log.Fatal(err)
	}

	err = closeES(idx, logESWriter)
	if err != nil {
		log.Fatal(err)
	}
}

func closeES(idx elasticsearch.IIndex, logWriter pzsyslog.Writer) error {
	err := logWriter.Close()
	if err != nil {
		log.Fatal(err)
	}

	return idx.Close()
}

func setupES(sys *piazza.SystemConfig) (elasticsearch.IIndex, pzsyslog.Writer, pzsyslog.Writer, error) {
	var idx *elasticsearch.Index
	loggerIndex, err := pzsyslog.GetRequiredEnvVars()
	if err != nil {
		log.Fatalln(err)
	}
	{
		pwd, err := os.Getwd()
		if err != nil {
			return nil, nil, nil, err
		}
		esURL, err := sys.GetURL(piazza.PzElasticSearch)
		if err != nil {
			return nil, nil, nil, err
		}

		type ScriptRes struct {
			Status  string `json:"status"`
			Message string `json:"message"`
			Mapping string `json:"mapping"`
		}

		format := func(dat []byte) []byte {
			re := regexp.MustCompile(`\r?\n?\t`)
			return bytes.TrimPrefix([]byte(re.ReplaceAllString(string(dat), "")), []byte("\xef\xbb\xbf"))
		}

		log.Println("Running init script...")
		outDat, err := exec.Command("bash", pwd+"/db/000-CreateLoggerIndex.sh", "piazzalogger", esURL).Output()
		if err != nil {
			return nil, nil, nil, err
		}
		outDat = format(outDat)
		scriptRes := ScriptRes{}
		if err = json.Unmarshal(outDat, &scriptRes); err != nil {
			log.Fatalln(err)
		}
		if scriptRes.Status != "success" {
			log.Fatalf("Script failed: [%s]\n", scriptRes.Message)
		}
		if scriptRes.Message != "" {
			log.Println(" ", scriptRes.Message)
		}
		if idx, err = elasticsearch.NewIndex(sys, loggerIndex, ""); err != nil {
			log.Fatalln(err)
		}
		if scriptRes.Mapping != "" {
			inter, err := piazza.StructStringToInterface(scriptRes.Mapping)
			if err != nil {
				log.Fatalln(err)
			}
			var scriptMap, esMap map[string]interface{}
			var ok bool
			if scriptMap, ok = inter.(map[string]interface{}); !ok {
				log.Fatalf("Schema [LogData] on alias [%s] in script is not type map[string]interface{}\n", loggerIndex)
			}
			if inter, err = idx.GetMapping("LogData"); err != nil {
				log.Fatalln(err)
			}
			if esMap, ok = inter.(map[string]interface{}); !ok {
				log.Fatalf("Schema [LogData] on alias [%s] on elasticsearch is not type map[string]interface{}\n", loggerIndex)
			}
			if !reflect.DeepEqual(scriptMap, esMap) {
				log.Fatalf("Schema [LogData] on alias [%s] on elasticsearch does not match the mapping provided\n", loggerIndex)
			}
		}
	}

	logEsWriter := pzsyslog.NewElasticWriter(idx, pzsyslog.LoggerType)
	if _, err = logEsWriter.CreateIndex(); err != nil {
		return idx, nil, nil, err
	}

	return idx, logEsWriter, &pzsyslog.StdoutWriter{}, nil
}
