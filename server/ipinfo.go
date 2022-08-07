package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

func (l *LogApiServer) getIPINFO(ip string) IPInfo {
	url := fmt.Sprintf("https://ipinfo.io/%s/json", ip)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("cache-control", "no-cache")

	res, _ := http.DefaultClient.Do(req)

	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)

	ipdata := IPInfo{}
	err := json.Unmarshal(body, &ipdata)
	if err != nil {
		log.Printf("WARN: log line unmarshall failure %s", err.Error())
	}
	return ipdata
}
