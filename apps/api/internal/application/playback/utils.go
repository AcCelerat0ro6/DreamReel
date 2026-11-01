package applicationplayback

import domainplayback "DreamReel/internal/domain/playback"

func normalizeConfig(config *domainplayback.Config, platform string, networkType string) *domainplayback.Config {
	normalized := *config
	normalized.Platform = domainplayback.NormalizePlatform(normalized.Platform)
	normalized.NetworkType = domainplayback.NormalizeNetworkType(normalized.NetworkType)
	if normalized.Platform == "" {
		normalized.Platform = platform
	}
	if normalized.NetworkType == "" {
		normalized.NetworkType = networkType
	}
	if normalized.PreloadCount <= 0 {
		normalized.PreloadCount = domainplayback.DefaultPreloadCount
	}
	if normalized.PreloadCount > domainplayback.MaxPreloadLimit {
		normalized.PreloadCount = domainplayback.MaxPreloadLimit
	}
	if normalized.BufferMs <= 0 {
		normalized.BufferMs = domainplayback.DefaultBufferMs
	}
	return &normalized
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return domainplayback.DefaultPreloadCount
	}
	if limit > domainplayback.MaxPreloadLimit {
		return domainplayback.MaxPreloadLimit
	}
	return limit
}
