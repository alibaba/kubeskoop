package util

import (
	"encoding/json"

	log "github.com/sirupsen/logrus"
)

func ToJSONString(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		log.Errorf("error marshal json: %v", err)
		return ""
	}
	return string(data)
}
