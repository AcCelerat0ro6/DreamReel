package applicationplayback

import domainplayback "DreamReel/internal/domain/playback"

type PreloadResult struct {
	Items []*domainplayback.PreloadVideo
}

type QoSReportResult struct {
	Report  *domainplayback.QoSReport
	Created bool
}
