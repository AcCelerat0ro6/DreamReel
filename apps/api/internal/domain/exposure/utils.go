package domainexposure

func isSupportedEventType(eventType string) bool {
	switch eventType {
	case EventTypeExposed, EventTypePlay, EventTypeComplete, EventTypeSkip:
		return true
	default:
		return false
	}
}
