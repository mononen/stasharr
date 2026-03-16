package prowlarr_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mononen/stasharr/internal/clients/prowlarr"
)

func TestSearch_MapsResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-Api-Key") != "testkey" {
			t.Errorf("missing or wrong api key header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{
				"title": "Studio.Scene.Title.2024.01.15",
				"size": 1500000000,
				"publishDate": "2024-01-15T00:00:00Z",
				"indexer": "NZBGeek",
				"downloadUrl": "http://example.com/nzb/123",
				"guid": "abc-guid-123"
			},
			{
				"title": "Another.Release.2024",
				"size": 800000000,
				"publishDate": "0001-01-01T00:00:00Z",
				"indexer": "DrunkenSlug",
				"downloadUrl": "http://example.com/nzb/456",
				"guid": "def-guid-456"
			}
		]`))
	}))
	defer srv.Close()

	c := prowlarr.New(srv.URL, "testkey")
	results, err := c.Search(context.Background(), "Studio Scene Title", 10)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	r0 := results[0]
	if r0.Title != "Studio.Scene.Title.2024.01.15" {
		t.Errorf("Title: got %q", r0.Title)
	}
	if r0.SizeBytes != 1500000000 {
		t.Errorf("SizeBytes: got %d", r0.SizeBytes)
	}
	if r0.IndexerName != "NZBGeek" {
		t.Errorf("IndexerName: got %q", r0.IndexerName)
	}
	if r0.DownloadURL != "http://example.com/nzb/123" {
		t.Errorf("DownloadURL: got %q", r0.DownloadURL)
	}
	if r0.NzbID != "abc-guid-123" {
		t.Errorf("NzbID: got %q", r0.NzbID)
	}
	if r0.PublishDate == nil {
		t.Fatal("PublishDate should be non-nil")
	}
	want := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	if !r0.PublishDate.Equal(want) {
		t.Errorf("PublishDate: got %v, want %v", r0.PublishDate, want)
	}

	// Zero publish date should be mapped to nil pointer.
	if results[1].PublishDate != nil {
		t.Errorf("zero publishDate should map to nil pointer")
	}
}

func TestSearch_QueryParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("query") != "test query" {
			t.Errorf("query param: got %q", q.Get("query"))
		}
		if q.Get("limit") != "25" {
			t.Errorf("limit param: got %q", q.Get("limit"))
		}
		if q.Get("type") != "search" {
			t.Errorf("type param: got %q", q.Get("type"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := prowlarr.New(srv.URL, "k")
	_, err := c.Search(context.Background(), "test query", 25)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearch_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := prowlarr.New(srv.URL, "bad")
	_, err := c.Search(context.Background(), "q", 10)
	if err == nil {
		t.Fatal("expected error for non-200")
	}
	var se *prowlarr.StatusError
	if !asError(err, &se) {
		t.Fatalf("expected StatusError, got %T: %v", err, err)
	}
	if se.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode: got %d", se.StatusCode)
	}
}

func TestSearch_UnparseableResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	c := prowlarr.New(srv.URL, "k")
	_, err := c.Search(context.Background(), "q", 10)
	if err == nil {
		t.Fatal("expected parse error")
	}
	var pe *prowlarr.ParseError
	if !asError(err, &pe) {
		t.Fatalf("expected ParseError, got %T: %v", err, err)
	}
}

func TestFetchNZB_ReturnsBytes(t *testing.T) {
	nzbContent := []byte(`<?xml version="1.0"?><nzb xmlns="http://www.newzbin.com/DTD/2003/nzb"></nzb>`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "testkey" {
			t.Errorf("missing api key header on FetchNZB")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(nzbContent)
	}))
	defer srv.Close()

	c := prowlarr.New("http://unused", "testkey")
	// Use the test server URL as the download URL directly.
	got, err := c.FetchNZB(context.Background(), srv.URL+"/nzb/123")
	if err != nil {
		t.Fatalf("FetchNZB error: %v", err)
	}
	if string(got) != string(nzbContent) {
		t.Errorf("FetchNZB: got %q, want %q", got, nzbContent)
	}
}

func TestFetchNZB_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := prowlarr.New("http://unused", "k")
	_, err := c.FetchNZB(context.Background(), srv.URL+"/nzb/bad")
	if err == nil {
		t.Fatal("expected error")
	}
	var se *prowlarr.StatusError
	if !asError(err, &se) {
		t.Fatalf("expected StatusError, got %T", err)
	}
	if se.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode: got %d", se.StatusCode)
	}
}

func TestPing_ReturnsIndexerCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/indexer" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1},{"id":2},{"id":3}]`))
	}))
	defer srv.Close()

	c := prowlarr.New(srv.URL, "k")
	msg, err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping error: %v", err)
	}
	if msg != "ok: 3 indexer(s) configured" {
		t.Errorf("Ping: got %q", msg)
	}
}

// asError is a helper to avoid importing errors in tests; uses type assertion loop.
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
