package interfacehttpvideo

import (
	applicationvideo "DreamReel/internal/application/video"
	domainvideo "DreamReel/internal/domain/video"
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

type rankingStub struct {
	sort          string
	limit, offset int
	fail          bool
}

func (r *rankingStub) ListModelRankings(_ context.Context, sort string, limit, offset int) ([]domainvideo.ModelRanking, error) {
	r.sort, r.limit, r.offset = sort, limit, offset
	if r.fail {
		return nil, errors.New("private database failure")
	}
	if offset > 2 {
		return nil, nil
	}
	return []domainvideo.ModelRanking{{ModelName: "A", VideoCount: 2, HotScore: 30}, {ModelName: "B", VideoCount: 1}}, nil
}

func TestModelRankingAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &rankingStub{}
	g := gin.New()
	g.GET("/api/model-rankings", NewRanking(applicationvideo.NewRanking(repo)).List)
	request := func(query string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		g.ServeHTTP(rec, httptest.NewRequest("GET", "/api/model-rankings"+query, nil))
		return rec
	}
	rec := request("?limit=1&offset=1&sort=likes")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"rank":2`) || !strings.Contains(rec.Body.String(), `"has_more":true`) || strings.Contains(rec.Body.String(), `"model_name":"B"`) {
		t.Fatalf("pagination: %s", rec.Body.String())
	}
	if repo.sort != "likes" || repo.limit != 2 || repo.offset != 1 {
		t.Fatalf("query: %+v", repo)
	}
	request("?limit=999")
	if repo.sort != "hot" || repo.limit != 101 {
		t.Fatalf("defaults and limit cap: %+v", repo)
	}
	rec = request("?offset=100")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"items":[]`) || !strings.Contains(rec.Body.String(), `"has_more":false`) {
		t.Fatalf("empty: %s", rec.Body.String())
	}
	for _, query := range []string{"?sort=unknown", "?sort=hot%3BDROP", "?limit=0", "?limit=x", "?offset=-1", "?offset=x", "?offset=10001"} {
		if rec := request(query); rec.Code != 400 {
			t.Fatalf("accepted %s: %d", query, rec.Code)
		}
	}
	repo.fail = true
	rec = request("")
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "private database") {
		t.Fatalf("error response: %s", rec.Body.String())
	}
}
