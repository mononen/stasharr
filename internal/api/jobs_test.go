package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mononen/stasharr/internal/db/queries"
)

// mockQuerier is a partial implementation of queries.Querier for testing.
// Non-overridden methods return an error.
type mockQuerier struct {
	queries.Querier // embed interface; provides zero-value for unset fields

	getJobFn           func(context.Context, uuid.UUID) (queries.Job, error)
	createJobFn        func(context.Context, queries.CreateJobParams) (queries.Job, error)
	selectSearchResult func(context.Context, queries.SelectSearchResultParams) (queries.SearchResult, error)
	updateJobStatus    func(context.Context, queries.UpdateJobStatusParams) (queries.Job, error)
}

func (m *mockQuerier) GetJob(ctx context.Context, id uuid.UUID) (queries.Job, error) {
	if m.getJobFn != nil {
		return m.getJobFn(ctx, id)
	}
	return queries.Job{}, errors.New("GetJob: not implemented in mock")
}

func (m *mockQuerier) CreateJob(ctx context.Context, arg queries.CreateJobParams) (queries.Job, error) {
	if m.createJobFn != nil {
		return m.createJobFn(ctx, arg)
	}
	return queries.Job{}, errors.New("CreateJob: not implemented in mock")
}

func (m *mockQuerier) CreateBatchJob(ctx context.Context, arg queries.CreateBatchJobParams) (queries.BatchJob, error) {
	return queries.BatchJob{}, errors.New("CreateBatchJob: not implemented in mock")
}

func (m *mockQuerier) SelectSearchResult(ctx context.Context, arg queries.SelectSearchResultParams) (queries.SearchResult, error) {
	if m.selectSearchResult != nil {
		return m.selectSearchResult(ctx, arg)
	}
	return queries.SearchResult{}, errors.New("SelectSearchResult: not implemented in mock")
}

func (m *mockQuerier) UpdateJobStatus(ctx context.Context, arg queries.UpdateJobStatusParams) (queries.Job, error) {
	if m.updateJobStatus != nil {
		return m.updateJobStatus(ctx, arg)
	}
	return queries.Job{}, errors.New("UpdateJobStatus: not implemented in mock")
}

// --- URL validation tests (POST /api/v1/jobs) ---

// testCreateJobApp builds a Fiber app with a mock querier.
// CreateJob always returns an error so valid-URL tests result in 500
// (not 400), confirming validation passed.
func testCreateJobApp() *fiber.App {
	mock := &mockQuerier{
		createJobFn: func(_ context.Context, _ queries.CreateJobParams) (queries.Job, error) {
			return queries.Job{}, errors.New("db unavailable in test")
		},
	}
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Post("/jobs", handleCreateJobWith(mock, nil))
	return app
}

func postJobBody(t *testing.T, jobURL, jobType string) *bytes.Buffer {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"url": jobURL, "type": jobType})
	return bytes.NewBuffer(b)
}

func TestCreateJob_ValidSceneURL(t *testing.T) {
	app := testCreateJobApp()
	req := httptest.NewRequest(http.MethodPost, "/jobs",
		postJobBody(t, "https://stashdb.org/scenes/abc-123-def", "scene"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	// Validation passes; DB call will fail because app is nil → 500.
	// We only care it is NOT 400.
	if resp.StatusCode == http.StatusBadRequest {
		var body map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&body)
		t.Fatalf("expected validation to pass, got 400: %v", body)
	}
}

func TestCreateJob_ValidPerformerURL(t *testing.T) {
	app := testCreateJobApp()
	req := httptest.NewRequest(http.MethodPost, "/jobs",
		postJobBody(t, "https://stashdb.org/performers/performer-slug-456", "performer"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode == http.StatusBadRequest {
		var body map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&body)
		t.Fatalf("expected validation to pass, got 400: %v", body)
	}
}

func TestCreateJob_InvalidURL(t *testing.T) {
	app := testCreateJobApp()
	req := httptest.NewRequest(http.MethodPost, "/jobs",
		postJobBody(t, "https://example.com/not-stashdb", "scene"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreateJob_WrongType(t *testing.T) {
	app := testCreateJobApp()
	// Scene URL but declared as performer → mismatch
	req := httptest.NewRequest(http.MethodPost, "/jobs",
		postJobBody(t, "https://stashdb.org/scenes/abc-123", "performer"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreateJob_InvalidType(t *testing.T) {
	app := testCreateJobApp()
	req := httptest.NewRequest(http.MethodPost, "/jobs",
		postJobBody(t, "https://stashdb.org/scenes/abc-123", "unknown"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// --- Approve job tests ---

func testApproveApp(q queries.Querier) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Post("/jobs/:id/approve", handleApproveJobWith(q))
	return app
}

func TestApproveJob_Returns409WhenNotAwaitingReview(t *testing.T) {
	jobID := uuid.New()
	mock := &mockQuerier{
		getJobFn: func(_ context.Context, id uuid.UUID) (queries.Job, error) {
			return queries.Job{
				ID:     jobID,
				Status: "resolved", // not awaiting_review
				Type:   "scene",
			}, nil
		},
	}

	app := testApproveApp(mock)

	body, _ := json.Marshal(map[string]string{"result_id": uuid.New().String()})
	req := httptest.NewRequest(http.MethodPost, "/jobs/"+jobID.String()+"/approve",
		bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409, got %d", resp.StatusCode)
	}

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)
	errMap, ok := respBody["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected error envelope")
	}
	if errMap["code"] != "INVALID_STATUS" {
		t.Errorf("expected code INVALID_STATUS, got %v", errMap["code"])
	}
}

func TestApproveJob_Returns200WhenAwaitingReview(t *testing.T) {
	jobID := uuid.New()
	resultID := uuid.New()

	mock := &mockQuerier{
		getJobFn: func(_ context.Context, id uuid.UUID) (queries.Job, error) {
			return queries.Job{
				ID:     jobID,
				Status: "awaiting_review",
				Type:   "scene",
			}, nil
		},
		selectSearchResult: func(_ context.Context, arg queries.SelectSearchResultParams) (queries.SearchResult, error) {
			return queries.SearchResult{
				ID:    resultID,
				JobID: jobID,
				SelectedBy: pgtype.Text{String: "user", Valid: true},
			}, nil
		},
		updateJobStatus: func(_ context.Context, arg queries.UpdateJobStatusParams) (queries.Job, error) {
			return queries.Job{
				ID:     jobID,
				Status: "approved",
			}, nil
		},
	}

	app := testApproveApp(mock)

	body, _ := json.Marshal(map[string]string{"result_id": resultID.String()})
	req := httptest.NewRequest(http.MethodPost, "/jobs/"+jobID.String()+"/approve",
		bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)
	if respBody["status"] != "approved" {
		t.Errorf("expected status approved, got %v", respBody["status"])
	}
}

// --- extractEntityID unit tests ---

func TestExtractEntityID(t *testing.T) {
	cases := []struct {
		url      string
		jobType  string
		wantID   string
		wantOK   bool
	}{
		{"https://stashdb.org/scenes/abc-123", "scene", "abc-123", true},
		{"https://stashdb.org/performers/perf-slug", "performer", "perf-slug", true},
		{"https://stashdb.org/studios/studio-slug", "studio", "studio-slug", true},
		{"https://stashdb.org/scenes/abc-123", "performer", "", false},
		{"https://example.com/scenes/abc", "scene", "", false},
		{"https://stashdb.org/scenes/abc-123", "unknown", "", false},
	}

	for _, tc := range cases {
		id, ok := extractEntityID(tc.url, tc.jobType)
		if ok != tc.wantOK {
			t.Errorf("extractEntityID(%q, %q): ok=%v, want %v", tc.url, tc.jobType, ok, tc.wantOK)
			continue
		}
		if ok && id != tc.wantID {
			t.Errorf("extractEntityID(%q, %q): id=%q, want %q", tc.url, tc.jobType, id, tc.wantID)
		}
	}
}
