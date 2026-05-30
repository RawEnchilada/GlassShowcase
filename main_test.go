package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// ── Test DB setup ─────────────────────────────────────────────────────────────
//
// Tests share a single real MariaDB connection (DATABASE_URL or TEST_DATABASE_URL).
// Each test truncates the ratings table before running.
// Tests must not run in parallel.
//
// Set TEST_DATABASE_URL to a MariaDB DSN before running, e.g.:
//   TEST_DATABASE_URL="root:secret@tcp(127.0.0.1:3306)/testdb" go test ./...

var testDB *sql.DB

func getTestDB(t *testing.T) *sql.DB {
	t.Helper()
	if testDB != nil {
		return testDB
	}

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL (or DATABASE_URL) to run DB tests")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	testDB = db
	return testDB
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db := getTestDB(t)
	if _, err := db.Exec(`DELETE FROM ratings`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return db
}

func newTestMux(db *sql.DB) *http.ServeMux {
	limiter := newRateLimiter()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth(db))
	mux.HandleFunc("GET /api/ratings", handleGetRatings(db))
	mux.HandleFunc("POST /api/rate", handlePostRate(db, limiter))
	return mux
}

func postRate(t *testing.T, mux *http.ServeMux, projectID, rating, ip, ua string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"projectId": projectID, "rating": rating})
	req := httptest.NewRequest(http.MethodPost, "/api/rate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ip
	req.Header.Set("User-Agent", ua)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

func getRatings(t *testing.T, mux *http.ServeMux, ip, ua string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/ratings", nil)
	req.RemoteAddr = ip
	req.Header.Set("User-Agent", ua)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

// ── Health ────────────────────────────────────────────────────────────────────

func TestHealth_OK(t *testing.T) {
	db := newTestDB(t)
	mux := newTestMux(db)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Errorf("want ok=true, got %v", resp["ok"])
	}
}

func TestHealth_ClosedDB(t *testing.T) {
	// Use a fresh closed connection rather than closing the shared testDB.
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL (or DATABASE_URL) to run DB tests")
	}
	db, _ := sql.Open("mysql", dsn)
	db.Close()

	mux := newTestMux(db)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// ── GET /api/ratings ──────────────────────────────────────────────────────────

func TestGetRatings_EmptyDB(t *testing.T) {
	db := newTestDB(t)
	mux := newTestMux(db)

	rr := getRatings(t, mux, "1.2.3.4:0", "TestBrowser/1.0")

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	var resp struct {
		Ratings map[string]ratingCounts `json:"ratings"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Ratings) != len(projectIDs) {
		t.Errorf("want %d projects, got %d", len(projectIDs), len(resp.Ratings))
	}
	for id, rc := range resp.Ratings {
		if rc.Likes != 0 || rc.Dislikes != 0 {
			t.Errorf("project %s: want 0/0, got %d/%d", id, rc.Likes, rc.Dislikes)
		}
		if rc.Mine != "" {
			t.Errorf("project %s: want mine='', got %q", id, rc.Mine)
		}
	}
}

func TestGetRatings_ReflectsMine(t *testing.T) {
	db := newTestDB(t)
	mux := newTestMux(db)

	ip, ua := "1.2.3.4:0", "TestBrowser/1.0"
	rr := postRate(t, mux, "squid", "like", ip, ua)
	if rr.Code != http.StatusCreated {
		t.Fatalf("setup post: want 201, got %d", rr.Code)
	}

	rr = getRatings(t, mux, ip, ua)
	var resp struct {
		Ratings map[string]ratingCounts `json:"ratings"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp.Ratings["squid"].Mine != "like" {
		t.Errorf("want mine=like, got %q", resp.Ratings["squid"].Mine)
	}
	if resp.Ratings["squid"].Likes != 1 {
		t.Errorf("want 1 like, got %d", resp.Ratings["squid"].Likes)
	}
}

func TestGetRatings_DifferentUserSeesNoMine(t *testing.T) {
	db := newTestDB(t)
	mux := newTestMux(db)

	postRate(t, mux, "squid", "like", "1.1.1.1:0", "AgentA/1.0")

	rr := getRatings(t, mux, "2.2.2.2:0", "AgentB/1.0")
	var resp struct {
		Ratings map[string]ratingCounts `json:"ratings"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp.Ratings["squid"].Mine != "" {
		t.Errorf("different user should have mine='', got %q", resp.Ratings["squid"].Mine)
	}
	if resp.Ratings["squid"].Likes != 1 {
		t.Errorf("want 1 like visible to all, got %d", resp.Ratings["squid"].Likes)
	}
}

// ── POST /api/rate ────────────────────────────────────────────────────────────

func TestPostRate_Like(t *testing.T) {
	db := newTestDB(t)
	mux := newTestMux(db)

	rr := postRate(t, mux, "squid", "like", "1.2.3.4:0", "TestBrowser/1.0")

	if rr.Code != http.StatusCreated {
		t.Errorf("want 201, got %d: %s", rr.Code, rr.Body)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["accepted"] != true {
		t.Errorf("want accepted=true")
	}
	if resp["mine"] != "like" {
		t.Errorf("want mine=like, got %v", resp["mine"])
	}
}

func TestPostRate_Dislike(t *testing.T) {
	db := newTestDB(t)
	mux := newTestMux(db)

	rr := postRate(t, mux, "forge", "dislike", "1.2.3.4:0", "TestBrowser/1.0")

	if rr.Code != http.StatusCreated {
		t.Errorf("want 201, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["mine"] != "dislike" {
		t.Errorf("want mine=dislike, got %v", resp["mine"])
	}
}

func TestPostRate_DuplicateReturns409(t *testing.T) {
	db := newTestDB(t)
	mux := newTestMux(db)

	ip, ua := "1.2.3.4:0", "TestBrowser/1.0"
	postRate(t, mux, "squid", "like", ip, ua)
	rr := postRate(t, mux, "squid", "like", ip, ua)

	if rr.Code != http.StatusConflict {
		t.Errorf("want 409 on duplicate, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["accepted"] != false {
		t.Errorf("want accepted=false on duplicate")
	}
}

func TestPostRate_CountsAccumulate(t *testing.T) {
	db := newTestDB(t)
	mux := newTestMux(db)

	postRate(t, mux, "squid", "like", "1.1.1.1:0", "AgentA")
	postRate(t, mux, "squid", "like", "2.2.2.2:0", "AgentB")
	rr := postRate(t, mux, "squid", "dislike", "3.3.3.3:0", "AgentC")

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp["likes"] != float64(2) {
		t.Errorf("want 2 likes, got %v", resp["likes"])
	}
	if resp["dislikes"] != float64(1) {
		t.Errorf("want 1 dislike, got %v", resp["dislikes"])
	}
}

func TestPostRate_InvalidProject(t *testing.T) {
	db := newTestDB(t)
	mux := newTestMux(db)

	rr := postRate(t, mux, "notaproject", "like", "1.2.3.4:0", "TestBrowser/1.0")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid project, got %d", rr.Code)
	}
}

func TestPostRate_InvalidRating(t *testing.T) {
	db := newTestDB(t)
	mux := newTestMux(db)

	rr := postRate(t, mux, "squid", "meh", "1.2.3.4:0", "TestBrowser/1.0")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid rating, got %d", rr.Code)
	}
}

func TestPostRate_MalformedJSON(t *testing.T) {
	db := newTestDB(t)
	mux := newTestMux(db)

	req := httptest.NewRequest(http.MethodPost, "/api/rate", bytes.NewBufferString("not json"))
	req.RemoteAddr = "1.2.3.4:0"
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 for bad JSON, got %d", rr.Code)
	}
}

func TestPostRate_EmptyBody(t *testing.T) {
	db := newTestDB(t)
	mux := newTestMux(db)

	req := httptest.NewRequest(http.MethodPost, "/api/rate", bytes.NewBufferString("{}"))
	req.RemoteAddr = "1.2.3.4:0"
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 for empty fields, got %d", rr.Code)
	}
}

// ── Rate limiter ──────────────────────────────────────────────────────────────
// These tests are pure in-memory — no DB needed.

func TestRateLimiter_AllowsUpToLimit(t *testing.T) {
	r := newRateLimiter()
	for i := 0; i < rateLimitPosts; i++ {
		if !r.allow("1.2.3.4:0") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiter_BlocksAfterLimit(t *testing.T) {
	r := newRateLimiter()
	for i := 0; i < rateLimitPosts; i++ {
		r.allow("1.2.3.4:0")
	}
	if r.allow("1.2.3.4:0") {
		t.Error("should be blocked after limit")
	}
}

func TestRateLimiter_IndependentPerIP(t *testing.T) {
	r := newRateLimiter()
	for i := 0; i < rateLimitPosts; i++ {
		r.allow("1.1.1.1:0")
	}
	if !r.allow("2.2.2.2:0") {
		t.Error("different IP should not be rate limited")
	}
}

func TestRateLimiter_ResetsAfterWindow(t *testing.T) {
	r := newRateLimiter()
	r.mu.Lock()
	r.buckets["1.2.3.4:0"] = &rateBucket{
		windowStart: time.Now().Add(-rateLimitWindow - time.Second),
		count:       rateLimitPosts,
	}
	r.mu.Unlock()

	if !r.allow("1.2.3.4:0") {
		t.Error("should be allowed after window expires")
	}
}

// ── User ID ───────────────────────────────────────────────────────────────────

func TestGetUserId_Stable(t *testing.T) {
	id1 := getUserId("1.2.3.4:1234", "Mozilla/5.0")
	id2 := getUserId("1.2.3.4:1234", "Mozilla/5.0")
	if id1 != id2 {
		t.Error("same inputs should produce same user ID")
	}
}

func TestGetUserId_DifferentIP(t *testing.T) {
	id1 := getUserId("1.2.3.4:0", "Mozilla/5.0")
	id2 := getUserId("5.6.7.8:0", "Mozilla/5.0")
	if id1 == id2 {
		t.Error("different IPs should produce different user IDs")
	}
}

func TestGetUserId_DifferentUA(t *testing.T) {
	id1 := getUserId("1.2.3.4:0", "Mozilla/5.0")
	id2 := getUserId("1.2.3.4:0", "curl/7.0")
	if id1 == id2 {
		t.Error("different user agents should produce different user IDs")
	}
}
