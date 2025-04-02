package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/mmcloughlin/geohash"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	apiPort   = flag.String("apiPort", "2113", "Port for API Server")
	cacheFile = flag.String("cache", "./database/cache.json", "Cache JSON File Location for IP DATA")
	apiOnly   = flag.Bool("apiOnly", false, "Do not attempt to open a log file, just listen to api")
	logFile   = flag.String("log", "", "Log File Path")
	database  = flag.String("db", "", "DB File Path")
	loki      = flag.String("loki", "https://loki.whickerx.info", "loki url https://loki.whickerx.info")
	lokiOrg   = flag.String("lokiOrg", "wst", "X-Scope-OrgID header value")
)

type metrics struct {
	metrics map[string]prometheus.Gauge
	mu      sync.Mutex
}

type clients struct {
	clients map[string]*KongClients
	mu      sync.Mutex
}

type GrafanaQuery struct {
	PanelID int `json:"panelId"`
	Range   struct {
		From time.Time `json:"from"`
		To   time.Time `json:"to"`
		Raw  struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"raw"`
	} `json:"range"`
	RangeRaw struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"rangeRaw"`
	Interval      string `json:"interval"`
	IntervalMs    int    `json:"intervalMs"`
	MaxDataPoints int    `json:"maxDataPoints"`
	Targets       []struct {
		Target  string `json:"target"`
		RefID   string `json:"refId"`
		Payload struct {
			Additional string `json:"additional"`
		} `json:"payload,omitempty"`
	} `json:"targets"`
	AdhocFilters []struct {
		Key      string `json:"key"`
		Operator string `json:"operator"`
		Value    string `json:"value"`
	} `json:"adhocFilters"`
}

type LogApiServer struct {
	ts             []*TimeSeries
	clients        clients
	ignoreIpRanges map[string]bool
	logFile        *string
	rateLimited    bool
	database       *string
	metrics        metrics
}

type TimeSeries struct {
	Target     string      `json:"target"`
	Datapoints [][]float64 `json:"datapoints"`
}

type KongClients struct {
	Key       string  `json:"key"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Name      string  `json:"name"`
}

func NewLogApiServer() *LogApiServer {
	flag.Parse()
	log.Printf("Database Flag: %s", *database)
	log.Printf("Log Flag: %s", *logFile)
	ts := make([]*TimeSeries, 0)

	clients, err := readIPInfoCache(*cacheFile)
	if err != nil {
		log.Panic(err)
	}

	localIPRanges := make(map[string]bool, 0)
	idx := 0
	for idx < len(strings.Split(*ignoreIPRanges, ",")) {
		localIPRanges[strings.Split(*ignoreIPRanges, ",")[idx]] = true
		idx++
	}
	return &LogApiServer{
		ts:             ts,
		clients:        *clients,
		logFile:        logFile,
		ignoreIpRanges: localIPRanges,
		rateLimited:    false,
		database:       database,
		metrics:        metrics{metrics: map[string]prometheus.Gauge{}, mu: sync.Mutex{}},
	}
}

func (l *LogApiServer) StartServer() {
	http.HandleFunc("/query", l.timeseriesHandler)
	http.HandleFunc("/search", l.searchDemoHandler)
	http.HandleFunc("/countries", l.clientHandler)
	http.HandleFunc("/log", l.log)
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		err := l.initDatabase()
		if err != nil {
			log.Println(err)
		}
	}()

	if !*apiOnly {
		if *logFile != "" {
			go func() {
				err := l.readLog(*l.logFile)
				if err != nil {
					log.Println(err)
				}
			}()
		}
	}
	http.ListenAndServe(fmt.Sprintf(":%s", *apiPort), nil)
}
func (l *LogApiServer) initDatabase() error {
	if *l.database == "" {
		return nil
	}
	data, err := os.Open(*l.database)
	if err != nil {
		return err
	}
	fileScanner := bufio.NewScanner(data)
	fileScanner.Split(bufio.ScanLines)
	var wg sync.WaitGroup
	ch := make(chan []byte)
	// start the workers
	for t := 0; t < 40; t++ {
		wg.Add(1)
		go l.saveToDB(ch, &wg)
	}
	for fileScanner.Scan() {
		//log.Printf("reading line: %d\n", line)
		text := fileScanner.Text()
		if len(text) == 0 {
			continue
		}

		ch <- []byte(text)
	}
	close(ch)
	wg.Wait()
	return data.Close()
}

func (l *LogApiServer) saveToDB(ch chan []byte, waitgroup *sync.WaitGroup) {
	// cnosume a line
	for text := range ch {
		// do work
		var logLine KongLogItem
		err := json.Unmarshal(text, &logLine)
		if err != nil {
			log.Printf("error: %s on log line:%s\n", err.Error(), text)
		}
		secondsSince := int64(time.Now().Unix()) - logLine.StartedAt/1000
		if secondsSince > 604800 {
			log.Printf("skippimg %s hit us %s ago\n", logLine.ClientIP, secondsToHuman(int(secondsSince)))
			if logLine.Request.Headers["host"] != nil && strings.Contains(logLine.Request.Headers["host"].(string), "jf.beau.cf") {
				logLine.StartedAt = time.Now().UnixMilli()
			} else {
				continue
			}

		}
		err = l.processLogLine(logLine)
		if err != nil {
			log.Printf("error: %s on log line:%s\n", err.Error(), string(text))
		}
	}
	waitgroup.Done()
}

func (l *LogApiServer) log(w http.ResponseWriter, req *http.Request) {
	var kongLogLine []KongLogItem
	buf, err := ioutil.ReadAll(req.Body)
	if err != nil {
		log.Println("error decrypting query")
		w.WriteHeader(http.StatusInternalServerError)
		return

	}
	err = writeKongHits(string(buf))
	if err != nil {
		log.Println(fmt.Sprintf("Payload form Error: %s", err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	err = json.Unmarshal(buf, &kongLogLine)
	if err != nil {
		log.Println(fmt.Sprintf("Payload form Error: %s", err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	for _, item := range kongLogLine {
		err = l.processLogLine(item)
		if err != nil {
			log.Println(fmt.Sprintf("Payload form Error: %s", err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(200)
	return

}

func (l *LogApiServer) clientHandler(w http.ResponseWriter, req *http.Request) {
	log.Println(req.URL)
	clientArray := make([]*KongClients, 0)
	for _, v := range l.clients.clients {
		clientArray = append(clientArray, v)
	}
	js, err := json.Marshal(clientArray)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Println(string(js))
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
	return
}
func postLog(body []byte) {
	url := "http://192.168.1.232:80/logs"

	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Add("Host", "ipresolver")
	http.DefaultClient.Do(req)
	return
}
func (l *LogApiServer) timeseriesHandler(w http.ResponseWriter, req *http.Request) {
	log.Println(req.URL)
	var query GrafanaQuery
	buf, err := ioutil.ReadAll(req.Body)
	if err != nil {
		log.Println("error decrypting query")
		w.WriteHeader(http.StatusInternalServerError)
		return

	}

	err = json.Unmarshal(buf, &query)
	if err != nil {
		log.Println(fmt.Sprintf("Payload form Error: %s", err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	from := &query.Range.From
	to := &query.Range.To

	queryTS := make([]*TimeSeries, 0)
	queryMapTS := make(map[string]*TimeSeries, 0)
	for _, v := range l.ts {
		datapointTime := v.Datapoints[0][1]
		datapointTimeDur := time.Unix(int64(datapointTime), 0)
		if from.Unix() <= datapointTimeDur.Unix() &&
			to.Unix() >= datapointTimeDur.Unix() {
			if queryMapTS[v.Target] == nil {
				queryMapTS[v.Target] = v
				continue
			}
			queryMapTS[v.Target].Datapoints[0][0] = queryMapTS[v.Target].Datapoints[0][0] + 1
		}
	}
	for _, v := range queryMapTS {
		queryTS = append(queryTS, v)
	}

	js, err := json.Marshal(queryTS)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Println(string(js))
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
	return
}

func (l *LogApiServer) tsDemoHandler(w http.ResponseWriter, req *http.Request) {
	log.Println(req.URL)
	payload := `[{"target": "SE", "datapoints": [[183255.0, 1450754220000]]},{"target": "US", "datapoints": [[192224.0, 1450754220000]]}]`
	log.Println(payload)
	//payload := `{"series": [
	//{
	//  "name": "logins.count",
	//  "tags": {
	//    "geohash": "9wvfgzurfzb"
	//  },
	//  "columns": [
	//    "time",
	//    "metric"
	//  ],
	//  "values": [
	//    [
	//      1529762933815,
	//      75.654324173059
	//    ]
	//  ]
	//}
	//]}`
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(payload))

	return
}

func (l *LogApiServer) clientDemoHandler(w http.ResponseWriter, req *http.Request) {
	log.Println(req.URL)
	payload := `
[
  {
    "key": "SE",
    "latitude": 60.128161,
    "longitude": 18.643501,
    "name": "Sweden"
  },
  {
    "key": "US",
    "latitude": 37.09024,
    "longitude": -95.712891,
    "name": "United States"
  }
]
`
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	fmt.Print(payload)
	w.Write([]byte(payload))
	return
}

func (l *LogApiServer) searchDemoHandler(w http.ResponseWriter, req *http.Request) {
	log.Println(req.URL)
	payload := `
["boob"]
`
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	fmt.Print(payload)
	w.Write([]byte(payload))
	return
}

func (L *LogApiServer) registerMetric(key string, lat float64, lng float64, name, route, service, host string, seenAt float64) {
	if len(route) == 0 {
		route = "Unknown"
	}
	if len(service) == 0 {
		service = "Unknown"
	}
	if L.metrics.metrics[key] == nil {
		L.metrics.metrics[key] = promauto.NewGauge(prometheus.GaugeOpts{
			Name: "kong_client",
			Help: "Client hitting kong server",
			ConstLabels: prometheus.Labels{
				"target":  key,
				"geohash": geohash.Encode(lat, lng),
				"place":   name,
				"route":   route,
				"service": service,
				"host":    host,
			},
		})
		L.metrics.metrics[key].Set(0)
	}
	metricLastSeenKey := fmt.Sprintf("%s_lastseen", key)
	if L.metrics.metrics[metricLastSeenKey] == nil {
		L.metrics.metrics[metricLastSeenKey] = promauto.NewGauge(prometheus.GaugeOpts{
			Name: "kong_client_lastseen",
			Help: "Client last hit the kong server",
			ConstLabels: prometheus.Labels{
				"target":  key,
				"geohash": geohash.Encode(lat, lng),
				"place":   name,
				"route":   route,
				"service": service,
				"host":    host,
			},
		})
	}
	L.metrics.metrics[key].Add(1)
	L.metrics.metrics[metricLastSeenKey].Set(seenAt)
}
