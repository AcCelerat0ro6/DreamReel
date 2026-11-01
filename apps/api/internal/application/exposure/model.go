package applicationexposure

import domainexposure "DreamReel/internal/domain/exposure"

type RecordViewEventResult struct {
	Event    *domainexposure.ViewEvent
	Exposure *domainexposure.Exposure
}
