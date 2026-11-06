package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestAIGCPublishAndFeed(t *testing.T) {
	env := newTestEnv(t)
	_, token := env.registerAndLogin(t, "aigc_creator", "pass123456", "AI Creator")
	headers := map[string]string{"Authorization": "Bearer " + token}
	base := func() map[string]any {
		return map[string]any{"title": "AIGC work", "media_url": "/uploads/test.mp4", "cover_url": "/uploads/test.jpg"}
	}
	payload := base()
	payload["model_name"] = " Model v2 "
	payload["model_params"] = " seed=42 "
	payload["ai_style_tag"] = " 水墨 "
	payload["origin_video_id"] = 12
	rec := doJSONRequest(t, env.engine, http.MethodPost, "/api/videos", payload, headers)
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish: %d %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	check := func(item map[string]any) {
		t.Helper()
		for key, want := range map[string]any{"model_name": "Model v2", "model_params": "seed=42", "ai_style_tag": "水墨", "origin_video_id": float64(12)} {
			if item[key] != want {
				t.Fatalf("%s: got %v want %v", key, item[key], want)
			}
		}
	}
	check(created)
	rec = doJSONRequest(t, env.engine, http.MethodGet, fmt.Sprintf("/api/videos/%.0f", created["id"]), nil, nil)
	var detail map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	check(detail)
	for _, path := range []string{"/api/feed-items?scene=timeline", "/api/feed-items?scene=hot"} {
		rec = doJSONRequest(t, env.engine, http.MethodGet, path, nil, nil)
		var page struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK || len(page.Items) != 1 {
			t.Fatalf("feed: %d %s", rec.Code, rec.Body.String())
		}
		check(page.Items[0])
	}
	for _, fields := range []map[string]any{{}, {"model_name": " ", "model_params": "\n", "ai_style_tag": " "}} {
		payload = base()
		for k, v := range fields {
			payload[k] = v
		}
		rec = doJSONRequest(t, env.engine, http.MethodPost, "/api/videos", payload, headers)
		if rec.Code != http.StatusCreated {
			t.Fatalf("ordinary publish: %s", rec.Body.String())
		}
		var item map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"model_name", "model_params", "ai_style_tag", "origin_video_id"} {
			if _, ok := item[key]; ok {
				t.Fatalf("empty field %s was returned", key)
			}
		}
	}
	for _, fields := range []map[string]any{{"origin_video_id": 0}, {"origin_video_id": -1}, {"model_name": strings.Repeat("中", 22)}, {"model_params": strings.Repeat("x", 1025)}, {"ai_style_tag": strings.Repeat("x", 33)}} {
		payload = base()
		for k, v := range fields {
			payload[k] = v
		}
		rec = doJSONRequest(t, env.engine, http.MethodPost, "/api/videos", payload, headers)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid AIGC accepted: %v %d", fields, rec.Code)
		}
	}
}
