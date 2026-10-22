package domaininteraction

import "strings"

func NormalizeActionType(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value != ActionTypeLike && value != ActionTypeFavorite {
		return "", ErrInvalidActionType
	}
	return value, nil
}

// Active 判断点赞或收藏当前是否处于有效状态。
func (a *Action) Active() bool {
	return a.Status == ActionStatusActive
}
