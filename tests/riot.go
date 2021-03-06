/*
   copyright 2018-2019 Banco Bilbao Vizcaya Argentaria, S.A.

   licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   you may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   withouT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   see the License for the specific language governing permissions and
   limitations under the License.
*/

package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/imdario/mergo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"

	"github.com/bbva/qed/api/metricshttp"
	"github.com/bbva/qed/client"
	"github.com/bbva/qed/log"
)

const riotHelp = `---
openapi: 3.0
info:
info:
  title: riot-workloader
  description: >
	this program runs as a single workloader (default) or as a server to receive
	"plans" (Config structs) through a small web API.

servers:
  - url: http://localhost:7700/

components:
  schemas:
    Config:
      type: object
      properties:
				endpoint:
					type: string
				apikey
					type: string
				insecure:
					type: bool
				kind:
					type: string
					enum: ["add", "membership", "incremental"]
				offload:
					type: bool
				profiling:
					type: bool
				incrementalDelta:
					type: integer
					minimum: 0
				offset:
					type: integer
					minimum: 0
				numRequests:
					type: integer
					minimum: 0
				maxGoRoutines:
					type: integer
					minimum: 0
				clusterSize:
					type: integer
					minimum: 0
		Plan:
			type: array
			items:
				type: array
				items:
					$ref: '#/components/schemas/Config'

paths:
  /:
    get:
      description: return this help and this openapi documentation.
      responses:
        200:
          description: this help.
          content:
            text/plain:
              schema:
                type: string

  /run:
    post:
      summary: Endpoint for receiving a single Config to run
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Config'
			examples:
			  simple: {"kind": "add"}
				advanced: {"kind": "incremental", "insecure":true, "endpoint": "https://qedserver:8800"}
				advanced: {"kind": "incremental", "insecure":true, "endpoint": "https://qedserver0:8800,qedserver1:8801"}

  /plan:
    post:
      summary: Endpoint for receiving a Plan to run
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Plan'
			examples:
			  single_plan:  [[{"kind": "add"}]]
			  secuential:  [[{"kind": "add"}], [{"kind": "membership"}]]
			  parallel:  [[{"kind": "add"}, {"kind": "membership"}]]
`

var (
	// Client

	RiotEventAdd = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "riot_event_add",
			Help: "Number of events added into the cluster.",
		},
	)
	RiotQueryMembership = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "riot_query_membership",
			Help: "Number of single events directly verified into the cluster.",
		},
	)
	RiotQueryIncremental = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "riot_query_incremental",
			Help: "Number of range of verified events queried into the cluster.",
		},
	)
	metricsList = []prometheus.Collector{
		RiotEventAdd,
		RiotQueryMembership,
		RiotQueryIncremental,
	}

	registerMetrics sync.Once
)

// Register all metrics.
func Register(r *prometheus.Registry) {
	// Register the metrics.
	registerMetrics.Do(
		func() {
			for _, metric := range metricsList {
				r.MustRegister(metric)
			}
		},
	)
}

type Riot struct {
	Config Config

	metricsServer      *http.Server
	prometheusRegistry *prometheus.Registry
}

type Config struct {
	// general conf
	Endpoint []string
	APIKey   string
	Insecure bool

	// stress conf
	Kind             string
	Offload          bool
	Profiling        bool
	IncrementalDelta uint
	Offset           uint
	NumRequests      uint
	MaxGoRoutines    uint
	ClusterSize      uint
}

type Plan [][]Config

type kind string

const (
	add         kind = "add"
	membership  kind = "membership"
	incremental kind = "incremental"
)

type Attack struct {
	kind           kind
	balloonVersion uint64

	config  Config
	client  *client.HTTPClient
	senChan chan Task
}

type Task struct {
	kind kind

	event               string
	key                 []byte
	version, start, end uint64
}

func main() {
	if err := newRiotCommand().Execute(); err != nil {
		os.Exit(-1)
	}
}

func newRiotCommand() *cobra.Command {
	// Input storage.
	var logLevel string
	var APIMode bool
	riot := Riot{}

	cmd := &cobra.Command{
		Use:   "riot",
		Short: "Stresser tool for qed server",
		Long:  riotHelp,
		PreRun: func(cmd *cobra.Command, args []string) {

			log.SetLogger("Riot", logLevel)

			if riot.Config.Profiling {
				go func() {
					log.Info("	* Starting Riot Profiling server")
					log.Info(http.ListenAndServe(":6060", nil))
				}()
			}

			if !APIMode && riot.Config.Kind == "" {
				log.Fatal("Argument `kind` is required")
			}

		},
		Run: func(cmd *cobra.Command, args []string) {
			riot.Start(APIMode)
		},
	}

	f := cmd.Flags()

	f.StringVarP(&logLevel, "log", "l", "debug", "Choose between log levels: silent, error, info and debug")
	f.BoolVar(&APIMode, "api", false, "Raise a HTTP api in port 7700")

	f.StringSliceVarP(&riot.Config.Endpoint, "endpoint", "e", []string{"127.0.0.1:8800"}, "The endopoint to make the load")
	f.StringVarP(&riot.Config.APIKey, "apikey", "k", "my-key", "The key to use qed servers")
	f.BoolVar(&riot.Config.Insecure, "insecure", false, "Allow self-signed TLS certificates")

	f.StringVar(&riot.Config.Kind, "kind", "", "The kind of load to execute")

	f.BoolVar(&riot.Config.Profiling, "profiling", false, "Enable Go profiling $ go tool pprof")
	f.UintVarP(&riot.Config.IncrementalDelta, "delta", "d", 1000, "Specify delta for the IncrementalProof")
	f.UintVar(&riot.Config.NumRequests, "n", 10e4, "Number of requests for the attack")
	f.UintVar(&riot.Config.MaxGoRoutines, "r", 10, "Set the concurrency value")
	f.UintVar(&riot.Config.Offset, "offset", 0, "The starting version from which we start the load")

	return cmd
}

func (riot *Riot) Start(APIMode bool) {

	r := prometheus.NewRegistry()
	Register(r)
	riot.prometheusRegistry = r
	metricsMux := metricshttp.NewMetricsHTTP(r)
	log.Debug("	* Starting Riot Metrics server")
	riot.metricsServer = &http.Server{Addr: ":17700", Handler: metricsMux}

	if APIMode {
		riot.Serve()
	} else {
		riot.RunOnce()
	}

}

func (riot *Riot) RunOnce() {
	newAttack(riot.Config)
}

func postReqSanitizer(w http.ResponseWriter, r *http.Request) (http.ResponseWriter, *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return w, r
	}

	if r.Body == nil {
		http.Error(w, "Please send a request body", http.StatusBadRequest)
	}

	return w, r
}
func (riot *Riot) MergeConf(newConf Config) Config {
	conf := riot.Config
	_ = mergo.Merge(&conf, newConf)
	return conf
}

func (riot *Riot) Serve() {

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, riotHelp)
	})
	mux.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		w, r = postReqSanitizer(w, r)

		var newConf Config
		err := json.NewDecoder(r.Body).Decode(&newConf)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		newAttack(riot.MergeConf(newConf))
	})

	mux.HandleFunc("/plan", func(w http.ResponseWriter, r *http.Request) {
		var wg sync.WaitGroup
		w, r = postReqSanitizer(w, r)

		var plan Plan
		err := json.NewDecoder(r.Body).Decode(&plan)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		for _, batch := range plan {
			for _, conf := range batch {
				wg.Add(1)
				go func(conf Config) {
					newAttack(riot.MergeConf(conf))
					wg.Done()
				}(conf)

			}
			wg.Wait()
		}
	})

	api := &http.Server{Addr: ":7700", Handler: mux}

	log.Debug("	* Starting Riot HTTP server")
	if err := api.ListenAndServe(); err != http.ErrServerClosed {
		log.Errorf("Can't start Riot API HTTP server: %s", err)
	}
}

func newAttack(conf Config) {

	// QED client
	transport := http.DefaultTransport.(*http.Transport)
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: conf.Insecure}
	httpClient := http.DefaultClient
	httpClient.Transport = transport
	client, err := client.NewHTTPClient(
		client.SetHttpClient(httpClient),
		client.SetURLs(conf.Endpoint[0], conf.Endpoint[1:]...),
		client.SetAPIKey(conf.APIKey),
		client.SetReadPreference(client.Any),
		client.SetAttemptToReviveEndpoints(true),
	)

	if err != nil {
		panic(err)
	}

	attack := Attack{
		client:         client,
		config:         conf,
		kind:           kind(conf.Kind),
		balloonVersion: uint64(conf.NumRequests + conf.Offset - 1),
	}

	if err := attack.client.Ping(); err != nil {
		panic(err)
	}

	attack.Run()
}

func (a *Attack) Run() {
	var wg sync.WaitGroup
	a.senChan = make(chan Task)

	for rID := uint(0); rID < a.config.MaxGoRoutines; rID++ {
		wg.Add(1)
		go func(rID uint) {
			for {
				task, ok := <-a.senChan
				if !ok {
					log.Debugf("!!! clos: %d", rID)
					wg.Done()
					return
				}

				switch task.kind {
				case add:
					log.Debugf(">>> add: %s", task.event)
					_, _ = a.client.Add(task.event)
					RiotEventAdd.Inc()
				case membership:
					log.Debugf(">>> mem: %s, %d", task.event, task.version)
					_, _ = a.client.Membership(task.key, task.version)
					RiotQueryMembership.Inc()
				case incremental:
					log.Debugf(">>> inc: %s", task.event)
					_, _ = a.client.Incremental(task.start, task.end)
					RiotQueryIncremental.Inc()
				}
			}
		}(rID)
	}

	for i := a.config.Offset; i < a.config.Offset+a.config.NumRequests; i++ {
		ev := fmt.Sprintf("event %d", i)
		a.senChan <- Task{
			kind:    a.kind,
			event:   ev,
			key:     []byte(ev),
			version: a.balloonVersion,
			start:   uint64(i),
			end:     uint64(i + a.config.IncrementalDelta),
		}
	}

	close(a.senChan)
	wg.Wait()
}
