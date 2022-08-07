package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
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

func readIPInfoCache(filename string) (map[string]*KongClients, error) {
	// Open our yamlFile
	jsonFile, err := os.Open(filename)
	// if we os.Open returns an error then handle it
	if err != nil {
		fmt.Println(err)
	}
	log.Println(fmt.Sprintf("Successfully Opened: %s", filename))
	defer jsonFile.Close()

	byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		return nil, err
	}

	var result map[string]*KongClients
	err = json.Unmarshal(byteValue, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
