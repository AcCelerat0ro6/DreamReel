package cache

import (
	"context"
	"encoding/json"
	"strconv"

	applicationrecommendation "DreamReel/internal/application/recommendation"
	domainrecommendation "DreamReel/internal/domain/recommendation"

	"github.com/redis/go-redis/v9"
)

type RecommendationSessionStore struct{ client *redis.Client }

// NewRecommendationSessionStore 复用 API 的 Redis 连接保存推荐会话。
func NewRecommendationSessionStore(client *redis.Client) *RecommendationSessionStore {
	return &RecommendationSessionStore{client: client}
}

func sessionKeys(id string) []string {
	prefix := "rec:session:{" + id + "}"
	return []string{prefix + ":meta", prefix + ":items", prefix + ":enqueued"}
}

func (s *RecommendationSessionStore) Read(ctx context.Context, id string, offset, count int) (applicationrecommendation.SessionState, []*domainrecommendation.Candidate, error) {
	keys := sessionKeys(id)
	var state applicationrecommendation.SessionState
	// 在同一事务中读取队列窗口及其版本，避免混用补池前后的长度。
	pipe := s.client.TxPipeline()
	meta := pipe.HGet(ctx, keys[0], "state")
	window := pipe.LRange(ctx, keys[1], int64(offset), int64(offset)+int64(count)-1)
	if _, err := pipe.Exec(ctx); err != nil {
		if err == redis.Nil {
			return state, nil, domainrecommendation.ErrCursorExpired
		}
		return state, nil, err
	}
	if err := json.Unmarshal([]byte(meta.Val()), &state); err != nil {
		return state, nil, err
	}
	items := make([]*domainrecommendation.Candidate, 0, len(window.Val()))
	for _, raw := range window.Val() {
		var item domainrecommendation.Candidate
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			return state, nil, err
		}
		items = append(items, &item)
	}
	return state, items, nil
}

func (s *RecommendationSessionStore) EnqueuedIDs(ctx context.Context, id string) ([]int64, error) {
	// 操作 `rec:session:{4f8a2c1b9d}:enqueued`
	values, err := s.client.SMembers(ctx, sessionKeys(id)[2]).Result()
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(values))
	for _, value := range values {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// 三个键共用会话过期时间。返回 0 表示其他请求已补池，-1 表示会话失效。
var appendSession = redis.NewScript(`
local version = redis.call('HGET', KEYS[1], 'version')
local expected = tonumber(ARGV[1])
if not version and expected ~= 0 then return -1 end
if version and tonumber(version) ~= expected then return 0 end
local ttl = tonumber(ARGV[3])
if version then
 ttl = redis.call('PTTL', KEYS[1])
 if ttl <= 0 then return -1 end
end
local size = tonumber(ARGV[4])
if redis.call('LLEN', KEYS[2]) ~= size or redis.call('SCARD', KEYS[3]) ~= size then return -1 end
for i = 5, #ARGV, 2 do
 redis.call('RPUSH', KEYS[2], ARGV[i+1])
 redis.call('SADD', KEYS[3], ARGV[i])
end
redis.call('HSET', KEYS[1], 'version', expected+1, 'state', ARGV[2])
for i = 1, 3 do redis.call('PEXPIRE', KEYS[i], ttl) end
return 1
`)

func (s *RecommendationSessionStore) Append(ctx context.Context, id string, expectedVersion int, next applicationrecommendation.SessionState, items []*domainrecommendation.Candidate) (bool, error) {
	meta, err := json.Marshal(next)
	if err != nil {
		return false, err
	}
	args := []any{expectedVersion, meta, applicationrecommendation.SessionTTL.Milliseconds(), next.Count - len(items)}
	for _, item := range items {
		raw, err := json.Marshal(item)
		if err != nil {
			return false, err
		}
		args = append(args, item.VideoID, raw)
	}
	code, err := appendSession.Run(ctx, s.client, sessionKeys(id), args...).Int()
	if err != nil {
		return false, err
	}
	if code < 0 {
		return false, domainrecommendation.ErrCursorExpired
	}
	return code == 1, nil
}
