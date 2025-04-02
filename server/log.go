package server

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/hpcloud/tail"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var ignoreIPRanges = flag.String("ignoreIPs", "192.168.1,192.168.0,172.17.,10.42.", "substring of IPs to ignore")

type X struct {
	Latencies struct {
		Request int     `json:"request"`
		Proxy   float64 `json:"proxy"`
		Kong    int     `json:"kong"`
	} `json:"latencies"`
	StartedAt   int64  `json:"started_at"`
	UpstreamURI string `json:"upstream_uri"`
	Route       struct {
		CreatedAt               int      `json:"created_at"`
		UpdatedAt               int      `json:"updated_at"`
		Protocols               []string `json:"protocols"`
		PathHandling            string   `json:"path_handling"`
		RequestBuffering        bool     `json:"request_buffering"`
		StripPath               bool     `json:"strip_path"`
		ID                      string   `json:"id"`
		ResponseBuffering       bool     `json:"response_buffering"`
		RegexPriority           int      `json:"regex_priority"`
		HTTPSRedirectStatusCode int      `json:"https_redirect_status_code"`
		PreserveHost            bool     `json:"preserve_host"`
		Hosts                   []string `json:"hosts"`
		Service                 struct {
			ID string `json:"id"`
		} `json:"service"`
		Name string `json:"name"`
		WsID string `json:"ws_id"`
	} `json:"route"`
	ClientIP string `json:"client_ip"`
	Service  struct {
		ReadTimeout    int    `json:"read_timeout"`
		CreatedAt      int    `json:"created_at"`
		UpdatedAt      int    `json:"updated_at"`
		Port           int    `json:"port"`
		ID             string `json:"id"`
		WsID           string `json:"ws_id"`
		ConnectTimeout int    `json:"connect_timeout"`
		Host           string `json:"host"`
		WriteTimeout   int    `json:"write_timeout"`
		Protocol       string `json:"protocol"`
		Name           string `json:"name"`
		Retries        int    `json:"retries"`
	} `json:"service"`
	CountryCode string `json:"countryCode,omitempty"`
	//Tries []struct {
	//	BalancerLatency int    `json:"balancer_latency"`
	//	Port            int    `json:"port"`
	//	BalancerStart   int64  `json:"balancer_start"`
	//	IP              string `json:"ip"`
	//} `json:"tries"`
	Request struct {
		Headers map[string]interface {
		} `json:"headers,omitempty"`
	} `json:"request,omitempty"`
	Response struct {
		//Headers struct {
		//	Vary                    string `json:"vary"`
		//	Date                    string `json:"date"`
		//	Via                     string `json:"via"`
		//	XKongProxyLatency       string `json:"x-kong-proxy-latency"`
		//	Server                  string `json:"server"`
		//	XKongUpstreamLatency    string `json:"x-kong-upstream-latency"`
		//	Upgrade                 string `json:"upgrade"`
		//	Connection              string `json:"connection"`
		//	StrictTransportSecurity string `json:"strict-transport-security"`
		//	SecWebsocketAccept      string `json:"sec-websocket-accept"`
		//} `json:"headers"`
		Status int `json:"status"`
		Size   int `json:"size"`
	} `json:"response"`
}
type KongLogItem struct {
	UpstreamStatus string `json:"upstream_status,omitempty"`
	UpstreamUri    string `json:"upstream_uri,omitempty"`
	CountryCode    string `json:"countryCode,omitempty"`
	Response       struct {
		Headers map[string]interface{} `json:"headers,omitempty"`
		Status  int                    `json:"status,omitempty"`
		Size    int                    `json:"size,omitempty"`
	} `json:"response,omitempty"`
	ClientIP string `json:"client_ip,omitempty"`
	Route    struct {
		RequestBuffering        bool     `json:"request_buffering,omitempty"`
		ResponseBuffering       bool     `json:"response_buffering,omitempty"`
		CreatedAt               int      `json:"created_at,omitempty"`
		UpdatedAt               int      `json:"updated_at,omitempty"`
		HttpsRedirectStatusCode int      `json:"https_redirect_status_code,omitempty"`
		Protocols               []string `json:"protocols,omitempty"`
		Name                    string   `json:"name,omitempty"`
		Service                 struct {
			Id string `json:"id,omitempty"`
		} `json:"service,omitempty"`
		StripPath     bool     `json:"strip_path,omitempty"`
		Id            string   `json:"id,omitempty"`
		PathHandling  string   `json:"path_handling,omitempty"`
		RegexPriority int      `json:"regex_priority,omitempty"`
		Hosts         []string `json:"hosts,omitempty"`
		PreserveHost  bool     `json:"preserve_host,omitempty"`
	} `json:"route,omitempty"`
	Service struct {
		ReadTimeout  int    `json:"read_timeout,omitempty"`
		CreatedAt    int    `json:"created_at,omitempty"`
		UpdatedAt    int    `json:"updated_at,omitempty"`
		Host         string `json:"host,omitempty"`
		Enabled      bool   `json:"enabled,omitempty"`
		Protocol     string `json:"protocol,omitempty"`
		WriteTimeout int    `json:"write_timeout,omitempty"`
		Name         string `json:"name,omitempty"`
		Port         int    `json:"port,omitempty"`
		//Tags           []interface{} `json:"tags,omitempty"`
		Id             string `json:"id,omitempty"`
		WsId           string `json:"ws_id,omitempty"`
		Retries        int    `json:"retries,omitempty"`
		ConnectTimeout int    `json:"connect_timeout,omitempty"`
	} `json:"service,omitempty"`
	StartedAt int64 `json:"started_at,omitempty"`
	Request   struct {
		Tls struct {
			Cipher       string `json:"cipher,omitempty"`
			Version      string `json:"version,omitempty"`
			ClientVerify string `json:"client_verify,omitempty"`
		} `json:"tls,omitempty"`
		Size        int                    `json:"size,omitempty"`
		Headers     map[string]interface{} `json:"headers,omitempty"`
		Querystring struct {
		} `json:"querystring,omitempty"`
		Uri    string `json:"uri,omitempty"`
		Url    string `json:"url,omitempty"`
		Method string `json:"method,omitempty"`
	} `json:"request,omitempty"`
}

type IPInfo struct {
	IP      string `json:"ip"`
	City    string `json:"city"`
	Region  string `json:"region"`
	Country string `json:"country"`
	Loc     string `json:"loc"`
	Org     string `json:"org"`
}

func (l *LogApiServer) readLog(logLocation string) error {
	t, err := tail.TailFile(logLocation, tail.Config{Follow: true})

	if t == nil || err != nil {
		return fmt.Errorf("an error has occured reading log: %s", logLocation)
	}

	for line := range t.Lines {
		if len(line.Text) == 0 {
			continue
		}
		jsonLogLine := KongLogItem{}
		err = json.Unmarshal([]byte(line.Text), &jsonLogLine)
		if err != nil {
			//log.Printf("WARN: log line unmarshall failure %s \n", err.Error())
			continue
		}
		err = l.processLogLine(jsonLogLine)
		if err != nil {
			continue
		}

	}

	return nil
}
func (l *LogApiServer) processLogLine(jsonLogLine KongLogItem) error {
	host := "unknown"
	if header, ok := jsonLogLine.Request.Headers["host"]; ok {
		host = header.(string)
	}
	line, err := json.Marshal(jsonLogLine)
	if err != nil {
		return err
	}
	secondsSince := int64(time.Now().Unix()) - jsonLogLine.StartedAt/1000
	clientIp := jsonLogLine.ClientIP
	if l.ignoreIP(clientIp) {
		//go pushLineToLoki(jsonLogLine.StartedAt, string(line), map[string]interface{}{"label": "kong", "host": host})
		return nil
	}
	log.Printf("{\"ip\":\"%s\", \"msg\":\"hit us %s ago\"}", clientIp, secondsToHuman(int(secondsSince)))
	if client, ok := l.clients.clients[jsonLogLine.ClientIP]; ok {
		cc := strings.Split(client.Name, "/")
		if len(cc) > 0 {
			jsonLogLine.CountryCode = cc[0]
		}
	}

	if l.clients.clients[jsonLogLine.ClientIP] == nil || (l.clients.clients[jsonLogLine.ClientIP] != nil && strings.Index(l.clients.clients[jsonLogLine.ClientIP].Name, "/") == 0) {
		l.clients.mu.Lock()
		log.Printf("IP not found in cache: %s", clientIp)
		location :=
			[]string{""}
		ipdata := IPInfo{}
		if !l.rateLimited {
			ipdata = l.getIPINFO(clientIp)
			location = strings.Split(ipdata.Loc, ",")
		}

		if len(location) != 2 {
			log.Printf("Couldnt find ipinfo.io data for %s", clientIp)
			log.Println("IPINFO response probably rate limited")
			l.rateLimited = true
			l.clients.clients[clientIp] = &KongClients{
				Key:       clientIp,
				Latitude:  0.0,
				Longitude: 0.0,
				Name:      fmt.Sprintf("%s/%s (%s)", "Unknown", "Unknown", clientIp),
			}
		} else {
			lat, _ := strconv.ParseFloat(location[0], 64)
			lng, _ := strconv.ParseFloat(location[1], 64)
			l.clients.clients[clientIp] = &KongClients{
				Key:       clientIp,
				Latitude:  lat,
				Longitude: lng,
				Name:      fmt.Sprintf("%s/%s (%s)", ipdata.Country, ipdata.City, clientIp),
			}
			err := writeKongClients(l.clients.clients)
			if err != nil {
				log.Panic(err)
			}
		}
		l.clients.mu.Unlock()
	}
	if l.clients.clients[clientIp] != nil {
		host := "unknown"
		if item, exist := jsonLogLine.Request.Headers["host"]; exist {
			host = item.(string)
		}
		l.metrics.mu.Lock()
		defer l.metrics.mu.Unlock()
		l.registerMetric(clientIp, l.clients.clients[clientIp].Latitude, l.clients.clients[clientIp].Longitude, l.clients.clients[clientIp].Name, jsonLogLine.Route.Name, jsonLogLine.Service.Name, host, float64(jsonLogLine.StartedAt/1000))
	}
	l.ts = append(l.ts, &TimeSeries{
		Target: clientIp,
		Datapoints: [][]float64{
			{1, float64(jsonLogLine.StartedAt / 1000)},
		},
	})
	go pushExternalLineToloki(jsonLogLine.StartedAt, host, string(line), jsonLogLine.CountryCode)

	return nil
}
func plural(count int, singular string) (result string) {
	if (count == 1) || (count == 0) {
		result = strconv.Itoa(count) + " " + singular + " "
	} else {
		result = strconv.Itoa(count) + " " + singular + "s "
	}
	return
}

func pushExternalLineToloki(timestamp int64, host, line, countryCode string) {
	labels := map[string]interface{}{"label": "logwatcher", "host": host, "countryCode": countryCode}
	pushLineToLoki(timestamp, line, labels)
}
func pushLineToLoki(timestamp int64, line string, labels map[string]interface{}) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("Error pushing to loki: %s\n", err)
		}
	}()
	logtimestamp := timestamp * 1000000
	weekAgo := time.Now().AddDate(0, 0, -7).UnixNano()
	if logtimestamp < weekAgo {
		return
	}
	//timestampStr := strconv.Itoa(logtimestamp)
	type payloadBody struct {
		Stream map[string]interface{} `json:"stream,omitempty"`
		Values [][]string             `json:"values,omitempty"`
	}

	type AutoGenerated struct {
		Streams []payloadBody `json:"streams,omitempty"`
	}
	pb := payloadBody{
		Stream: labels,
		Values: [][]string{{fmt.Sprintf("%d", logtimestamp), line}},
	}
	out, err := json.Marshal(AutoGenerated{Streams: []payloadBody{pb}})
	if err != nil {
		log.Println(err)
		return
	}
	payload := bytes.NewReader(out)
	url := fmt.Sprintf("%s/loki/api/v1/push", *loki)
	//log.Printf("pushing to %s", url)
	req, err := http.NewRequest("POST", url, payload)
	if err != nil {
		panic(err)
	}
	req.Header.Add("X-Scope-OrgID", *lokiOrg)
	req.Header.Add("content-type", "application/json")
	req.Header.Add("cache-control", "no-cache")
	//log.Println("do push")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	//log.Println("getting status code")
	if res.StatusCode != http.StatusNoContent {
		log.Printf("Log: %d weekAgo: %d\n", logtimestamp, weekAgo)
	}
}

func secondsToHuman(input int) (result string) {
	years := math.Floor(float64(input) / 60 / 60 / 24 / 7 / 30 / 12)
	seconds := input % (60 * 60 * 24 * 7 * 30 * 12)
	months := math.Floor(float64(seconds) / 60 / 60 / 24 / 7 / 30)
	seconds = input % (60 * 60 * 24 * 7 * 30)
	weeks := math.Floor(float64(seconds) / 60 / 60 / 24 / 7)
	seconds = input % (60 * 60 * 24 * 7)
	days := math.Floor(float64(seconds) / 60 / 60 / 24)
	seconds = input % (60 * 60 * 24)
	hours := math.Floor(float64(seconds) / 60 / 60)
	seconds = input % (60 * 60)
	minutes := math.Floor(float64(seconds) / 60)
	seconds = input % 60

	if years > 0 {
		result = plural(int(years), "year") + plural(int(months), "month") + plural(int(weeks), "week") + plural(int(days), "day") + plural(int(hours), "hour") + plural(int(minutes), "minute") + plural(int(seconds), "second")
	} else if months > 0 {
		result = plural(int(months), "month") + plural(int(weeks), "week") + plural(int(days), "day") + plural(int(hours), "hour") + plural(int(minutes), "minute") + plural(int(seconds), "second")
	} else if weeks > 0 {
		result = plural(int(weeks), "week") + plural(int(days), "day") + plural(int(hours), "hour") + plural(int(minutes), "minute") + plural(int(seconds), "second")
	} else if days > 0 {
		result = plural(int(days), "day") + plural(int(hours), "hour") + plural(int(minutes), "minute") + plural(int(seconds), "second")
	} else if hours > 0 {
		result = plural(int(hours), "hour") + plural(int(minutes), "minute") + plural(int(seconds), "second")
	} else if minutes > 0 {
		result = plural(int(minutes), "minute") + plural(int(seconds), "second")
	} else {
		result = plural(int(seconds), "second")
	}

	return
}

func (l *LogApiServer) ignoreIP(ip string) bool {
	if len(ip) == 0 {
		return true
	}
	// this is rubbish but quick solution for now
	for k, _ := range l.ignoreIpRanges {
		if strings.Index(ip, k) == 0 {
			return true
		}
	}
	return false
}
