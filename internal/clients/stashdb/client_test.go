package stashdb_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"

	"github.com/mononen/stasharr/internal/clients/stashdb"
)

// newTestClient creates a stashdb.Client pointed at the test server.
// It patches the private endpoint by replacing the real StashDB URL — since
// the client hardcodes the endpoint we instead use a custom RoundTripper.
func newTestClient(srv *httptest.Server) *stashdb.Client {
	return stashdb.NewWithEndpoint("testkey", nil, srv.URL)
}

func TestFindScene_MapsResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("ApiKey") != "testkey" {
			t.Errorf("ApiKey header: got %q", r.Header.Get("ApiKey"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type: got %q", r.Header.Get("Content-Type"))
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		vars, _ := req["variables"].(map[string]any)
		if vars["id"] != "scene-id-123" {
			t.Errorf("id variable: got %v", vars["id"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"data": {
				"findScene": {
					"id": "scene-id-123",
					"title": "Test Scene Title",
					"date": "2024-03-15",
					"duration": 4500,
					"studio": {"name": "Test Studio", "slug": "test-studio"},
					"performers": [
						{"performer": {"name": "Jane Doe", "slug": "jane-doe", "disambiguation": null}},
						{"performer": {"name": "John Smith", "slug": "john-smith", "disambiguation": "actor"}}
					],
					"tags": [{"name": "HD"}, {"name": "4K"}],
					"urls": [{"url": "https://example.com/scene/123", "type": "HOME"}]
				}
			}
		}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	scene, err := c.FindScene(context.Background(), "scene-id-123")
	if err != nil {
		t.Fatalf("FindScene error: %v", err)
	}

	if scene.ID != "scene-id-123" {
		t.Errorf("ID: got %q", scene.ID)
	}
	if scene.Title != "Test Scene Title" {
		t.Errorf("Title: got %q", scene.Title)
	}
	if scene.Date != "2024-03-15" {
		t.Errorf("Date: got %q", scene.Date)
	}
	if scene.DurationSeconds != 4500 {
		t.Errorf("DurationSeconds: got %d", scene.DurationSeconds)
	}
	if scene.StudioName != "Test Studio" {
		t.Errorf("StudioName: got %q", scene.StudioName)
	}
	if scene.StudioSlug != "test-studio" {
		t.Errorf("StudioSlug: got %q", scene.StudioSlug)
	}
	if len(scene.Performers) != 2 {
		t.Fatalf("Performers: got %d", len(scene.Performers))
	}
	if scene.Performers[0].Name != "Jane Doe" {
		t.Errorf("Performers[0].Name: got %q", scene.Performers[0].Name)
	}
	if scene.Performers[1].Disambiguation == nil || *scene.Performers[1].Disambiguation != "actor" {
		t.Errorf("Performers[1].Disambiguation: unexpected value")
	}
	if len(scene.Tags) != 2 || scene.Tags[0] != "HD" || scene.Tags[1] != "4K" {
		t.Errorf("Tags: got %v", scene.Tags)
	}
	if len(scene.RawResponse) == 0 {
		t.Error("RawResponse should be populated")
	}
}

func TestFindScene_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"findScene":null}}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.FindScene(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for null findScene")
	}
	var se *stashdb.StatusError
	if !asError(err, &se) {
		t.Fatalf("expected StatusError, got %T: %v", err, err)
	}
	if se.StatusCode != 404 {
		t.Errorf("StatusCode: got %d", se.StatusCode)
	}
}

func TestFindScene_GraphQLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"Unauthorized"}]}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.FindScene(context.Background(), "id")
	if err == nil {
		t.Fatal("expected error")
	}
	var pe *stashdb.ParseError
	if !asError(err, &pe) {
		t.Fatalf("expected ParseError, got %T: %v", err, err)
	}
}

func TestFindScene_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.FindScene(context.Background(), "id")
	if err == nil {
		t.Fatal("expected error")
	}
	var se *stashdb.StatusError
	if !asError(err, &se) {
		t.Fatalf("expected StatusError, got %T", err)
	}
	if se.StatusCode != http.StatusForbidden {
		t.Errorf("StatusCode: got %d", se.StatusCode)
	}
}

func TestFindPerformerScenes_Pagination(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		vars, _ := req["variables"].(map[string]any)
		input, _ := vars["input"].(map[string]any)
		page := int(input["page"].(float64))

		w.WriteHeader(http.StatusOK)
		if page == 1 {
			// First page: return 2 scenes, total count 3.
			_, _ = w.Write([]byte(`{
				"data": {
					"queryScenes": {
						"count": 3,
						"scenes": [
							{"id":"s1","title":"Scene 1","date":"","duration":0,"studio":null,"performers":[],"tags":[],"urls":[]},
							{"id":"s2","title":"Scene 2","date":"","duration":0,"studio":null,"performers":[],"tags":[],"urls":[]}
						]
					}
				}
			}`))
		} else {
			// Second page: return remaining 1 scene.
			_, _ = w.Write([]byte(`{
				"data": {
					"queryScenes": {
						"count": 3,
						"scenes": [
							{"id":"s3","title":"Scene 3","date":"","duration":0,"studio":null,"performers":[],"tags":[],"urls":[]}
						]
					}
				}
			}`))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv)
	scenes, err := c.FindPerformerScenes(context.Background(), "performer-id-1")
	if err != nil {
		t.Fatalf("FindPerformerScenes error: %v", err)
	}
	if len(scenes) != 3 {
		t.Errorf("expected 3 scenes across 2 pages, got %d", len(scenes))
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls for pagination, got %d", callCount)
	}
	if scenes[2].ID != "s3" {
		t.Errorf("scenes[2].ID: got %q", scenes[2].ID)
	}
}

func TestRateLimiter_IsInvoked(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"findScene":{"id":"x","title":"T","date":"","duration":0,"studio":null,"performers":[],"tags":[],"urls":[]}}}`))
	}))
	defer srv.Close()

	// A limiter with burst=1, rate=1/sec — enough for a single call in a test.
	limiter := rate.NewLimiter(1, 1)
	c := stashdb.NewWithEndpoint("key", limiter, srv.URL)
	_, err := c.FindScene(context.Background(), "x")
	if err != nil {
		t.Fatalf("FindScene error: %v", err)
	}
	// Verify the limiter was consumed (tokens drop below burst after one use).
	if limiter.Tokens() >= 1.0 {
		t.Error("rate limiter tokens should have been consumed")
	}
}

func TestPing_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"__typename":"Query"}}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping error: %v", err)
	}
}

func asError[T error](err error, target *T) bool {
	for err != nil {
		if v, ok := err.(T); ok {
			*target = v
			return true
		}
		type unwrapper interface{ Unwrap() error }
		if u, ok := err.(unwrapper); ok {
			err = u.Unwrap()
		} else {
			break
		}
	}
	return false
}
