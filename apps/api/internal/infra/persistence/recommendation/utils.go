package infrarecommendation

import (
	domainexposure "DreamReel/internal/domain/exposure"
	"encoding/json"
)

func decodeVector(content string) ([]float64, error) {
	var vector []float64
	if err := json.Unmarshal([]byte(content), &vector); err != nil {
		return nil, err
	}
	return vector, nil
}

func eventWeight(eventType string, watchMs int, completed bool) float64 {
	switch eventType {
	case domainexposure.EventTypeComplete:
		return 3
	case domainexposure.EventTypePlay:
		weight := 1 + float64(watchMs)/30000
		if weight > 2 {
			weight = 2
		}
		if completed {
			weight += 1
		}
		return weight
	default:
		return 1
	}
}

func positiveEventTypes() []string {
	return []string{
		domainexposure.EventTypePlay,
		domainexposure.EventTypeComplete,
	}
}
