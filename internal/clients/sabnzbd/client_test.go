package sabnzbd_test

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mononen/stasharr/internal/clients/sabnzbd"
)

func TestSubmitNZB_MultipartConstruction(t *testing.T) {
	nzbData := []byte(`<?xml version="1.0"?><nzb></nzb>`)
	var gotFilename, gotCat, gotNzbName string
	var gotFileBytes []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		q := r.URL.Query()
		if q.Get("mode") != "addfile" {
			t.Errorf("mode param: got %q", q.Get("mode"))
		}
		if q.Get("apikey") != "testkey" {
			t.Errorf("apikey param: got %q", q.Get("apikey"))
		}
		gotCat = q.Get("cat")
		gotNzbName = q.Get("nzbname")

		// Parse the multipart body.
		ct := r.Header.Get("Content-Type")
		_, params, err := mime.ParseMediaType(ct)
		if err != nil {
			t.Fatalf("bad content-type: %v", err)
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("multipart read error: %v", err)
			}
			gotFilename = part.FileName()
			gotFileBytes, _ = io.ReadAll(part)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":true,"nzo_ids":["SABnzbd_nzo_abc123"]}`))
	}))
	defer srv.Close()

	c := sabnzbd.New(srv.URL, "testkey", "stasharr")
	nzoID, err := c.SubmitNZB(context.Background(), nzbData, "My Scene Title")
	if err != nil {
		t.Fatalf("SubmitNZB error: %v", err)
	}
	if nzoID != "SABnzbd_nzo_abc123" {
		t.Errorf("nzoID: got %q", nzoID)
	}
	if gotCat != "stasharr" {
		t.Errorf("cat: got %q", gotCat)
	}
	if gotNzbName != "My Scene Title" {
		t.Errorf("nzbname: got %q", gotNzbName)
	}
	if gotFilename != "My Scene Title.nzb" {
		t.Errorf("filename: got %q", gotFilename)
	}
	if string(gotFileBytes) != string(nzbData) {
		t.Errorf("file bytes mismatch: got %q", gotFileBytes)
	}
}

func TestSubmitNZB_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":false,"error":"Invalid NZB file"}`))
	}))
	defer srv.Close()

	c := sabnzbd.New(srv.URL, "k", "cat")
	_, err := c.SubmitNZB(context.Background(), []byte(`bad`), "title")
	if err == nil {
		t.Fatal("expected error")
	}
	var ae *sabnzbd.APIError
	if !asError(err, &ae) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if ae.Message != "Invalid NZB file" {
		t.Errorf("APIError.Message: got %q", ae.Message)
	}
}

func TestGetQueue_StatusMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("mode") != "queue" {
			t.Errorf("mode param: got %q", r.URL.Query().Get("mode"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"queue": {
				"slots": [
					{
						"nzo_id": "SABnzbd_nzo_abc",
						"filename": "Some.Scene.Title",
						"status": "Downloading",
						"percentage": "62",
						"mb": "1000",
						"mbleft": "380"
					}
				]
			}
		}`))
	}))
	defer srv.Close()

	c := sabnzbd.New(srv.URL, "k", "cat")
	items, err := c.GetQueue(context.Background())
	if err != nil {
		t.Fatalf("GetQueue error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item := items[0]
	if item.NzoID != "SABnzbd_nzo_abc" {
		t.Errorf("NzoID: got %q", item.NzoID)
	}
	if item.Filename != "Some.Scene.Title" {
		t.Errorf("Filename: got %q", item.Filename)
	}
	if item.Status != "Downloading" {
		t.Errorf("Status: got %q", item.Status)
	}
	if item.Percentage != "62" {
		t.Errorf("Percentage: got %q", item.Percentage)
	}
	if item.MB != "1000" {
		t.Errorf("MB: got %q", item.MB)
	}
}

func TestGetHistory_StatusMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("mode") != "history" {
			t.Errorf("mode param: got %q", r.URL.Query().Get("mode"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"history": {
				"slots": [
					{
						"nzo_id": "SABnzbd_nzo_xyz",
						"name": "Completed.Scene",
						"status": "Completed",
						"size": "2.5 GB",
						"storage": "/downloads/complete/stasharr/Completed.Scene"
					},
					{
						"nzo_id": "SABnzbd_nzo_fail",
						"name": "Failed.Scene",
						"status": "Failed",
						"size": "1 GB",
						"storage": ""
					}
				]
			}
		}`))
	}))
	defer srv.Close()

	c := sabnzbd.New(srv.URL, "k", "cat")
	items, err := c.GetHistory(context.Background())
	if err != nil {
		t.Fatalf("GetHistory error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	completed := items[0]
	if completed.Status != "Completed" {
		t.Errorf("Status: got %q", completed.Status)
	}
	if completed.StoragePath != "/downloads/complete/stasharr/Completed.Scene" {
		t.Errorf("StoragePath: got %q", completed.StoragePath)
	}
	if completed.Size != "2.5 GB" {
		t.Errorf("Size: got %q", completed.Size)
	}

	failed := items[1]
	if failed.Status != "Failed" {
		t.Errorf("Status: got %q", failed.Status)
	}
}

func TestGetQueue_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := sabnzbd.New(srv.URL, "k", "cat")
	_, err := c.GetQueue(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var se *sabnzbd.StatusError
	if !asError(err, &se) {
		t.Fatalf("expected StatusError, got %T", err)
	}
	if se.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode: got %d", se.StatusCode)
	}
}

func TestDeleteJob_SendsCorrectParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("mode") != "queue" {
			t.Errorf("mode: got %q", q.Get("mode"))
		}
		if q.Get("name") != "delete" {
			t.Errorf("name: got %q", q.Get("name"))
		}
		if q.Get("value") != "SABnzbd_nzo_del123" {
			t.Errorf("value: got %q", q.Get("value"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":true}`))
	}))
	defer srv.Close()

	c := sabnzbd.New(srv.URL, "k", "cat")
	err := c.DeleteJob(context.Background(), "SABnzbd_nzo_del123")
	if err != nil {
		t.Fatalf("DeleteJob error: %v", err)
	}
}

func TestPing_ReturnsSABnzbdVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("mode") != "version" {
			t.Errorf("mode: got %q", r.URL.Query().Get("mode"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"version":"3.7.2"}`))
	}))
	defer srv.Close()

	c := sabnzbd.New(srv.URL, "k", "cat")
	msg, err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping error: %v", err)
	}
	if !strings.Contains(msg, "3.7.2") {
		t.Errorf("Ping: got %q, want version string", msg)
	}
}

func TestSubmitNZBURL_SendsCorrectParams(t *testing.T) {
	var gotMode, gotName, gotCat, gotNzbName string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		q := r.URL.Query()
		gotMode = q.Get("mode")
		gotName = q.Get("name")
		gotCat = q.Get("cat")
		gotNzbName = q.Get("nzbname")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":true,"nzo_ids":["SABnzbd_nzo_url123"]}`))
	}))
	defer srv.Close()

	directURL := "https://indexer.example.com/download?apikey=secret&id=12345"
	c := sabnzbd.New(srv.URL, "testkey", "stasharr")
	nzoID, err := c.SubmitNZBURL(context.Background(), directURL, "My Scene Title")
	if err != nil {
		t.Fatalf("SubmitNZBURL error: %v", err)
	}
	if nzoID != "SABnzbd_nzo_url123" {
		t.Errorf("nzoID: got %q", nzoID)
	}
	if gotMode != "addurl" {
		t.Errorf("mode: got %q, want %q", gotMode, "addurl")
	}
	if gotName != directURL {
		t.Errorf("name: got %q, want %q", gotName, directURL)
	}
	if gotCat != "stasharr" {
		t.Errorf("cat: got %q", gotCat)
	}
	if gotNzbName != "My Scene Title" {
		t.Errorf("nzbname: got %q", gotNzbName)
	}
}

func TestSubmitNZBURL_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":false,"error":"Invalid URL"}`))
	}))
	defer srv.Close()

	c := sabnzbd.New(srv.URL, "k", "cat")
	_, err := c.SubmitNZBURL(context.Background(), "https://indexer.example.com/dl?id=1", "title")
	if err == nil {
		t.Fatal("expected error")
	}
	var ae *sabnzbd.APIError
	if !asError(err, &ae) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if ae.Message != "Invalid URL" {
		t.Errorf("APIError.Message: got %q", ae.Message)
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
