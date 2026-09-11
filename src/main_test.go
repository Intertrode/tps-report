package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
)

func newTestServer(t *testing.T) *server {
	t.Helper()
	if os.Getenv("DB_HOST") == "" {
		t.Skip("DB_HOST not set; skipping DB integration test")
	}
	cfg := loadConfig()
	db := initDB(cfg.DSN())
	if err := db.Exec("TRUNCATE TABLE reports RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatalf("failed to reset reports table: %v", err)
	}
	return &server{db: db}
}

func TestHealthz(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.handleHealthz(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
}

func TestCreateAndGetReport(t *testing.T) {
	s := newTestServer(t)

	body := bytes.NewBufferString(`{"title":"Cover Sheet","author":"Peter","status":"missing"}`)
	req := httptest.NewRequest(http.MethodPost, "/reports", body)
	rec := httptest.NewRecorder()
	s.handleCreateReport(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create: got status %d, want 201; body=%s", rec.Code, rec.Body.String())
	}

	var created Report
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected non-zero ID from create")
	}

	idStr := strconv.Itoa(int(created.ID))
	getReq := httptest.NewRequest(http.MethodGet, "/reports/"+idStr, nil)
	getReq.SetPathValue("id", idStr)
	getRec := httptest.NewRecorder()
	s.handleGetReport(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("get: got status %d, want 200", getRec.Code)
	}

	var fetched Report
	if err := json.Unmarshal(getRec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if fetched.Title != "Cover Sheet" || fetched.Author != "Peter" {
		t.Fatalf("fetched record mismatch: %+v", fetched)
	}
}

func TestListReports(t *testing.T) {
	s := newTestServer(t)

	for _, title := range []string{"A", "B", "C"} {
		body := bytes.NewBufferString(`{"title":"` + title + `","author":"x","status":"draft"}`)
		req := httptest.NewRequest(http.MethodPost, "/reports", body)
		rec := httptest.NewRecorder()
		s.handleCreateReport(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("seed create %s: status %d", title, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/reports", nil)
	rec := httptest.NewRecorder()
	s.handleListReports(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list: got status %d, want 200", rec.Code)
	}

	var reports []Report
	if err := json.Unmarshal(rec.Body.Bytes(), &reports); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(reports) != 3 {
		t.Fatalf("got %d reports, want 3", len(reports))
	}
}
