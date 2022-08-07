package server

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/hpcloud/tail"
	"log"
	"math"
	"strconv"
	"strings"
	"time"
)

var ignoreIPRanges = flag.String("ignoreIPs", "192.168.1,192.168.0,172.17.", "Cache JSON File Location for IP DATA")

type KongLogItem struct {
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
	//Tries []struct {
	//	BalancerLatency int    `json:"balancer_latency"`
	//	Port            int    `json:"port"`
	//	BalancerStart   int64  `json:"balancer_start"`
	//	IP              string `json:"ip"`
	//} `json:"tries"`
	//Request struct {
	//Headers struct {
	//	SecWebsocketVersion    string `json:"sec-websocket-version"`
	//	SecWebsocketKey        string `json:"sec-websocket-key"`
	//	UserAgent              string `json:"user-agent"`
	//	SecWebsocketExtensions string `json:"sec-websocket-extensions"`
	//	//Host                   string `json:"host"`
	//	Cookie                 string `json:"cookie"`
	//	Origin                 string `json:"origin"`
	//	AcceptLanguage         string `json:"accept-language"`
	//	AcceptEncoding         string `json:"accept-encoding"`
	//	Upgrade                string `json:"upgrade"`
	//	Connection             string `json:"connection"`
	//	CacheControl           string `json:"cache-control"`
	//	Pragma                 string `json:"pragma"`
	//} `json:"headers"`
	//	TLS struct {
	//		ClientVerify string `json:"client_verify"`
	//		Version      string `json:"version"`
	//		Cipher       string `json:"cipher"`
	//	} `json:"tls"`
	//	URI         string `json:"uri"`
	//	Size        int    `json:"size"`
	//	Method      string `json:"method"`
	//	URL         string `json:"url"`
	//	Querystring struct {
	//	} `json:"querystring"`
	//} `json:"request"`
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
		//Size   int `json:"size"`
	} `json:"response"`
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
	secondsSince := int64(time.Now().Unix()) - jsonLogLine.StartedAt/1000
	clientIp := jsonLogLine.ClientIP
	if !l.ignoreIP(clientIp) {
		log.Printf("%s hit us %s ago", clientIp, secondsToHuman(int(secondsSince)))
		if l.clients[jsonLogLine.ClientIP] == nil || (l.clients[jsonLogLine.ClientIP] != nil && strings.Index(l.clients[jsonLogLine.ClientIP].Name, "/") == 0) {
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
				l.clients[clientIp] = &KongClients{
					Key:       clientIp,
					Latitude:  0.0,
					Longitude: 0.0,
					Name:      fmt.Sprintf("%s/%s (%s)", "Unknown", "Unknown", clientIp),
				}
			} else {
				lat, _ := strconv.ParseFloat(location[0], 64)
				lng, _ := strconv.ParseFloat(location[1], 64)
				l.clients[clientIp] = &KongClients{
					Key:       clientIp,
					Latitude:  lat,
					Longitude: lng,
					Name:      fmt.Sprintf("%s/%s (%s)", ipdata.Country, ipdata.City, clientIp),
				}
				err := writeKongClients(l.clients)
				if err != nil {
					log.Panic(err)
				}
			}
		}
		if l.clients[clientIp] != nil {
			l.registerMetric(clientIp, l.clients[clientIp].Latitude, l.clients[clientIp].Longitude, l.clients[clientIp].Name, jsonLogLine.Route.Name, jsonLogLine.Service.Name, float64(jsonLogLine.StartedAt/1000))
		}
		l.ts = append(l.ts, &TimeSeries{
			Target: clientIp,
			Datapoints: [][]float64{
				{1, float64(jsonLogLine.StartedAt / 1000)},
			},
		})
	}
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
