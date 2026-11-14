//go:build labperf

package performance

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appfeed "DreamReel/internal/application/feed"
	rec "DreamReel/internal/application/recommendation"
	appvideo "DreamReel/internal/application/video"
	feed "DreamReel/internal/domain/feed"
	"DreamReel/internal/infra/cache"
	frepo "DreamReel/internal/infra/persistence/feed"
	rrepo "DreamReel/internal/infra/persistence/recommendation"
	"github.com/redis/go-redis/v9"
	gm "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Counts completed GORM statements, including Rows/Raw, only inside each measured operation.
type sqlCounter struct {
	logger.Interface
	calls atomic.Int64
}

func (c *sqlCounter) Trace(context.Context, time.Time, func() (string, int64), error) { c.calls.Add(1) }

type trial struct {
	prepare    func()
	run        func(int)
	beforeEach func()
}

type suite struct {
	t     *testing.T
	out   *json.Encoder
	db    *gorm.DB
	count *sqlCounter
	redis *redis.Client
	cache *cache.FeedCache
	repo  *frepo.Repository
	ctx   context.Context
}

func require(ok bool, format string, args ...any) {
	if !ok {
		panic(fmt.Sprintf(format, args...))
	}
}
func check(err error) {
	if err != nil {
		panic(err)
	}
}

func (s *suite) pair(name string, n int, a, b trial) {
	s.t.Helper()
	for round := 1; round <= 4; round++ {
		order := []int{0, 1}
		if round%2 == 0 {
			slices.Reverse(order)
		}
		for _, side := range order {
			v := []trial{a, b}[side]
			if v.prepare != nil {
				v.prepare()
			}
			v.run(-1) // untimed, same path as a measured operation
			durations := make([]float64, n)
			calls := s.count.calls.Load()
			for i := range n {
				if v.beforeEach != nil {
					v.beforeEach()
				}
				start := time.Now()
				v.run(i)
				durations[i] = float64(time.Since(start).Nanoseconds()) / 1e6
			}
			check(s.out.Encode(map[string]any{"case": name, "variant": []string{"A", "B"}[side], "round": round,
				"sample_ms": durations, "sql": s.count.calls.Load() - calls}))
		}
	}
}

func TestLab(t *testing.T) {
	dir := os.Getenv("DREAMREEL_LAB_DIR")
	marker, err := os.ReadFile(filepath.Join(dir, "lab-owner"))
	if err != nil || string(marker) != "dreamreel-perf-v1" {
		t.Fatal("start via benchmarks/lab/run.py")
	}
	counter := &sqlCounter{Interface: logger.Discard}
	db, err := gorm.Open(gm.Open("root@unix("+filepath.Join(dir, "mysql.sock")+")/bench?parseTime=true&interpolateParams=true"), &gorm.Config{Logger: counter})
	check(err)
	pool, err := db.DB()
	check(err)
	defer pool.Close()
	pool.SetMaxOpenConns(32)
	pool.SetMaxIdleConns(16)
	r := redis.NewClient(&redis.Options{Network: "unix", Addr: filepath.Join(dir, "redis.sock"), DB: 2})
	defer r.Close()
	out, err := os.OpenFile(os.Getenv("LAB_COMPONENT_RESULTS"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	check(err)
	defer out.Close()
	s := suite{t, json.NewEncoder(out), db, counter, r, cache.NewFeedCache(r), frepo.New(db), context.Background()}
	s.pagination()
	s.cards()
	s.rankings()
	s.stampede()
	s.window()
	s.profile()
	s.fanout()
}

func (s *suite) pagination() {
	const depth = 9000
	var anchor feed.TimelineCursor
	check(s.db.Raw("SELECT id AS video_id,published_at FROM video WHERE status=2 ORDER BY published_at DESC,id DESC LIMIT 1 OFFSET ?", depth-1).Scan(&anchor).Error)
	const query = "SELECT id AS video_id,author_id,published_at FROM video WHERE status=2 AND published_at IS NOT NULL ORDER BY published_at DESC,id DESC LIMIT 11 OFFSET ?"
	var expected []*feed.FeedPageItem
	check(s.db.Raw(query, depth).Scan(&expected).Error)
	require(len(expected) == 11, "deep page incomplete")
	s.pair("deep_page", 240, trial{run: func(int) {
		var rows []*feed.FeedPageItem
		check(s.db.Raw(query, depth).Scan(&rows).Error)
		require(reflect.DeepEqual(rows, expected), "offset page mismatch")
	}}, trial{run: func(int) {
		rows, err := s.repo.ListTimelinePage(s.ctx, &anchor, 11)
		check(err)
		require(reflect.DeepEqual(rows, expected), "cursor page mismatch")
	}})
}

func (s *suite) cards() {
	ids := make([]int64, 20)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	expectedCards, err := s.repo.BatchGetFeedCards(s.ctx, ids)
	check(err)
	expectedStats, err := s.repo.BatchGetFeedStats(s.ctx, ids)
	check(err)
	expectedActions, err := s.repo.BatchGetViewerActions(s.ctx, 20000, ids)
	check(err)
	load := func(batch []int64) {
		cards, err := s.repo.BatchGetFeedCards(s.ctx, batch)
		check(err)
		stats, err := s.repo.BatchGetFeedStats(s.ctx, batch)
		check(err)
		actions, err := s.repo.BatchGetViewerActions(s.ctx, 20000, batch)
		check(err)
		for _, id := range batch {
			require(cards[id] != nil && cards[id].VideoID == id && reflect.DeepEqual(cards[id], expectedCards[id]) &&
				reflect.DeepEqual(stats[id], expectedStats[id]) && reflect.DeepEqual(actions[id], expectedActions[id]), "card mismatch %d", id)
		}
	}
	s.pair("card_assembly", 120, trial{run: func(int) {
		for i := range ids {
			load(ids[i : i+1])
		}
	}}, trial{run: func(int) { load(ids) }})
}

type pageCounter struct {
	feed.Repository
	pages atomic.Int64
}

func (r *pageCounter) ListTimelinePage(ctx context.Context, cursor *feed.TimelineCursor, limit int) ([]*feed.FeedPageItem, error) {
	r.pages.Add(1)
	return r.Repository.ListTimelinePage(ctx, cursor, limit)
}

func (s *suite) stampede() {
	// Workers share Redis and warm card/stat keys on both sides. Only strategy/group sharing changes.
	repo := &pageCounter{Repository: s.repo}
	const workers = 32
	services := make([]*appfeed.Service, workers)
	for i := range services {
		services[i] = appfeed.New(repo, appfeed.WithFeedCache(s.cache))
	}
	req := appfeed.FeedRequest{Scene: feed.SceneTimeline, Limit: 10}
	expected, err := services[0].GetFeed(s.ctx, req)
	check(err)
	for round := 1; round <= 4; round++ {
		for step := range 2 {
			shared := (round+step)%2 == 0
			times := []float64{}
			calls := int64(0)
			for wave := 0; wave < 40; wave++ {
				check(s.redis.Del(s.ctx, "feed:page:v1:timeline:limit:10:first").Err())
				startGate := make(chan struct{})
				var ready, done sync.WaitGroup
				ready.Add(workers)
				done.Add(workers)
				for i := range workers {
					svc := services[i]
					if shared {
						svc = services[0]
					}
					go func() {
						defer done.Done()
						ready.Done()
						<-startGate
						result, err := svc.GetFeed(s.ctx, req)
						check(err)
						require(reflect.DeepEqual(result, expected), "cold wave mismatch")
					}()
				}
				ready.Wait()
				before := repo.pages.Load()
				start := time.Now()
				close(startGate)
				done.Wait()
				times = append(times, float64(time.Since(start).Nanoseconds())/1e6)
				calls += repo.pages.Load() - before
			}
			variant := "A"
			if shared {
				variant = "B"
				require(calls == 40, "expected one page lookup per wave, got %d", calls)
			}
			check(s.out.Encode(map[string]any{"case": "cold_wave", "variant": variant, "round": round, "sample_ms": times, "page_queries": calls}))
		}
	}
}

func (s *suite) window() {
	end := time.Now().UTC().Truncate(time.Minute)
	for i := 1; i <= 10000; i++ {
		check(s.cache.AddHotScore(s.ctx, int64(i), i, end.Add(-time.Duration(i%60)*time.Minute)))
	}
	key := fmt.Sprintf("feed:hot:window:v1:%d", end.Unix())
	// Evict the derived window before starting each cold read's timer.
	read := func() {
		rows, err := s.cache.ListHotWindowPage(s.ctx, end, 0, 11)
		check(err)
		require(len(rows) == 11, "short hot page")
		for i, row := range rows {
			require(row.VideoID == int64(10000-i) && row.HotScore == 10000-i, "hot score mismatch")
		}
	}
	s.pair("hot_window", 120, trial{beforeEach: func() { check(s.redis.Del(s.ctx, key).Err()) }, run: func(int) { read() }}, trial{prepare: read, run: func(int) { read() }})
}

func (s *suite) profile() {
	repo := rrepo.New(s.db)
	input := rec.RefreshProfileInput{UserID: 20000, ViewEventID: 200, EventType: "complete", Completed: true}
	drop := func() { check(s.db.Exec("DELETE FROM user_interest_profile WHERE user_id=20000").Error) }
	drop()
	expected, ok, err := repo.LoadUserInterestVector(s.ctx, input.UserID)
	check(err)
	require(ok && len(expected) == 128, "missing reference profile")
	refresh := func() {
		result, err := rec.New(repo).RefreshUserInterestProfile(s.ctx, input)
		check(err)
		require(result.Updated && result.Profile.SampleCount == 200, "profile refresh mismatch")
	}
	read := func(int) {
		v, ok, err := repo.LoadUserInterestVector(s.ctx, input.UserID)
		check(err)
		require(ok && len(v) == len(expected), "missing profile")
		for i := range v {
			require(math.Abs(v[i]-expected[i]) < 1e-12, "profile differs")
		}
	}
	s.pair("interest_profile", 160, trial{prepare: drop, run: read}, trial{prepare: refresh, run: read})
	refresh()
}

// Counterfactual all-push uses the production worker and its normal 500-follower batches.
type alwaysPush struct{ *frepo.Repository }

func (r alwaysPush) CountFollowers(ctx context.Context, id int64) (int, error) {
	_, err := r.Repository.CountFollowers(ctx, id)
	return 0, err
}

func (s *suite) fanout() {
	push := appvideo.NewFanoutWorker(alwaysPush{s.repo}, nil, s.cache, nil)
	hybrid := appvideo.NewFanoutWorker(s.repo, nil, s.cache, nil)
	event := appvideo.PublishedEvent{AuthorID: 1, PublishedAt: time.Now().UTC().Truncate(time.Second)}
	id := int64(100000)
	run := func(w *appvideo.FanoutWorker) func(int) {
		return func(int) { id++; event.VideoID = id; check(w.HandleVideoPublished(s.ctx, &event)) }
	}
	s.pair("fanout_10000", 12, trial{run: run(push)}, trial{run: run(hybrid)})
	// Each branch published 4 * (12 measured + 1 warm-up) distinct IDs.
	for _, u := range []int{10001, 15000, 20000} {
		n, err := s.redis.ZCard(s.ctx, fmt.Sprintf("feed:following:inbox:v1:%d", u)).Result()
		check(err)
		require(n == 52, "fanout inbox %d has %d", u, n)
	}
	n, err := s.redis.ZCard(s.ctx, "feed:following:author:v1:1").Result()
	check(err)
	require(n == 52, "outbox size %d", n)
}
