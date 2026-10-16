package applicationrelation

import domainrelation "DreamReel/internal/domain/relation"

// FollowResult 是关注或取关后的关系状态和计数。
type FollowResult struct {
	UserID         int64
	TargetUserID   int64
	Status         int
	Following      bool
	FollowingCount int
	FollowerCount  int
}

// ListResult 是关注列表或粉丝列表的游标分页结果。
type ListResult struct {
	Items      []*domainrelation.UserItem
	NextCursor string
	HasMore    bool
}
