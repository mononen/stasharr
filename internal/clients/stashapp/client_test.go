package stashapp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mononen/stasharr/internal/clients/stashapp"
)

func TestFindSceneByPath_ReturnsTrue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("ApiKey") != "testkey" {
			t.Errorf("ApiKey header: got %q", r.Header.Get("ApiKey"))
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		vars, _ := req["variables"].(map[string]any)
		if vars["path"] != "/media/stash/Studio/Scene.mp4" {
			t.Errorf("path variable: got %v", vars["path"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"findScenes":{"count":1}}}`))
	}))
	defer srv.Close()

	c := stashapp.New(srv.URL, "testkey")
	exists, err := c.FindSceneByPath(context.Background(), "/media/stash/Studio/Scene.mp4")
	if err != nil {
		t.Fatalf("FindSceneByPath error: %v", err)
	}
	if !exists {
		t.Error("expected true when count=1")
	}
}

func TestFindSceneByPath_ReturnsFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"findScenes":{"count":0}}}`))
	}))
	defer srv.Close()

	c := stashapp.New(srv.URL, "k")
	exists, err := c.FindSceneByPath(context.Background(), "/media/stash/new.mp4")
	if err != nil {
		t.Fatalf("FindSceneByPath error: %v", err)
	}
	if exists {
		t.Error("expected false when count=0")
	}
}

func TestFindSceneByPath_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := stashapp.New(srv.URL, "bad")
	_, err := c.FindSceneByPath(context.Background(), "/some/path.mp4")
	if err == nil {
		t.Fatal("expected error")
	}
	var se *stashapp.StatusError
	if !asError(err, &se) {
		t.Fatalf("expected StatusError, got %T: %v", err, err)
	}
	if se.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode: got %d", se.StatusCode)
	}
}

func TestTriggerScan_ConstructsCorrectMutation(t *testing.T) {
	var capturedQuery string
	var capturedPaths []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		capturedQuery, _ = req["query"].(string)
		vars, _ := req["variables"].(map[string]any)
		input, _ := vars["input"].(map[string]any)
		rawPaths, _ := input["paths"].([]any)
		for _, p := range rawPaths {
			capturedPaths = append(capturedPaths, p.(string))
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"metadataScan":true}}`))
	}))
	defer srv.Close()

	c := stashapp.New(srv.URL, "k")
	err := c.TriggerScan(context.Background(), "/media/stash/Studio/scene.mp4")
	if err != nil {
		t.Fatalf("TriggerScan error: %v", err)
	}

	// Mutation should reference metadataScan.
	if !strings.Contains(capturedQuery, "metadataScan") {
		t.Errorf("query should contain metadataScan, got: %q", capturedQuery)
	}
	// Paths should be the parent directory of the given file.
	if len(capturedPaths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(capturedPaths))
	}
	if capturedPaths[0] != "/media/stash/Studio" {
		t.Errorf("path: got %q, want /media/stash/Studio", capturedPaths[0])
	}
}

func TestTriggerScan_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := stashapp.New(srv.URL, "k")
	err := c.TriggerScan(context.Background(), "/media/file.mp4")
	if err == nil {
		t.Fatal("expected error")
	}
	var se *stashapp.StatusError
	if !asError(err, &se) {
		t.Fatalf("expected StatusError, got %T", err)
	}
}

func TestPing_ReturnsVersionString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"version":{"version":"v0.27.1"}}}`))
	}))
	defer srv.Close()

	c := stashapp.New(srv.URL, "k")
	version, err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping error: %v", err)
	}
	if version != "v0.27.1" {
		t.Errorf("Ping: got %q, want v0.27.1", version)
	}
}

func TestFindSceneByPath_GraphQLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"scene_filter: path is invalid"}]}`))
	}))
	defer srv.Close()

	c := stashapp.New(srv.URL, "k")
	_, err := c.FindSceneByPath(context.Background(), "/bad")
	if err == nil {
		t.Fatal("expected error")
	}
	var pe *stashapp.ParseError
	if !asError(err, &pe) {
		t.Fatalf("expected ParseError, got %T: %v", err, err)
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
