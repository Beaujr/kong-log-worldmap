package server

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"sync"
)

func writeKongClients(data map[string]*KongClients) error {
	d1, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return writeConfig(d1, *cacheFile)
}

func writeConfig(data []byte, filename string) error {
	err := ioutil.WriteFile(filename, data, 0644)
	if err != nil {
		return err
	}
	return nil
}

func writeKongHits(data string) error {
	if *database == "" {
		return nil
	}
	f, err := os.OpenFile(*database,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
	}
	defer f.Close()
	if _, err := f.WriteString(fmt.Sprintf("%s\n", data)); err != nil {
		return err
	}
	return nil
}

func readIPInfoCache(filename string) (*clients, error) {
	var result map[string]*KongClients
	// Open our yamlFile
	jsonFile, err := os.Open(filename)
	// if we os.Open returns an error then handle it
	if err != nil {
		if os.IsNotExist(err) {
			return &clients{clients: map[string]*KongClients{}, mu: sync.Mutex{}}, nil
		} else {
			return nil, err
		}

	}
	log.Println(fmt.Sprintf("Successfully Opened: %s", filename))
	defer jsonFile.Close()

	byteValue, err := io.ReadAll(jsonFile)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(byteValue, &result)
	if err != nil {
		return nil, err
	}
	return &clients{clients: result, mu: sync.Mutex{}}, nil
}
