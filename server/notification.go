package server

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

func (l *LogApiServer) sendNotification(title string, message string, topic string) error {
	log.Printf("Notification: %s , %s", title, message)
	payload := strings.NewReader("{ \"title\": \"" + title + "\", \"body\":\"" + message + "\", \"image\": \"\"}")

	req, err := http.NewRequest("POST", fmt.Sprintf("%s%s", "https://nuc.beau.cf/fcm/send/", topic), payload)
	if err != nil {
		return err
	}
	req.Header.Add("content-type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, err = ioutil.ReadAll(res.Body)
	return err
}