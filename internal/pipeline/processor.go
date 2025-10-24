package pipeline

import (
	"encoding/json"
	"log"
)

func ProcessPacket(data map[string]interface{}) []string {
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Println("error marshaling:", err)
		return nil
	}
	return []string{string(jsonData)}
}
