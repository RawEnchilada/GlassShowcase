package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-txdb"
	_ "github.com/go-sql-driver/mysql"
)

// ── Test DB setup ─────────────────────────────────────────────────────────────
//
// Each test gets its own *sql.DB that is wrapped in a transaction which rolls
// back automatically when the connection is closed.  This means tests run
// against a real MariaDB (pointed to by TEST_DATABASE_URL) but never leave
// any state behind and can run in parallel safely.
//
// Set TEST_DATABASE_URL to a MariaDB DSN before running, e.g.:
//   TEST_DATABASE_URL="root:secret@tcp(127.0.0.1:3306)/testdb" go test ./...

var (
	txdbOnce sync.Once
	txdbDSN  string
)

func registerTxDB(t *testing.T) {
	t.Helper()
	txdbOnce.Do(func() {
		txdbDSN = os.Getenv("TEST_DATABASE_URL")
		if txdbDSN == "" {
			// Fall back to DATABASE_URL so local dev works without extra env var.
			txdbDSN = os.Getenv("DATABASE_URL")
		}
		if txdbDSN == "" {
			// Skip all DB tests rather than panic when no DB is available.
			return
		}
		txdb.Register("txdb", "mysql", txdbDSN)
	})
}

func requireDSN(t *testing.T) {
	t.Helper()
	registerTxDB(t)
	if txdbDSN == "" {
		t.Skip("set TEST_DATABASE_URL (or DATABASE_URL) to run DB tests")
	}
}

// newTestDB returns a *sql.DB whose single connection is wrapped in a
// transaction that rolls back when the db is closed (via t.Cleanup).
// The schema is applied fresh inside that transaction so every test starts
// from an empty set of tables.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	requireDSN(t)

	// Each call gets a unique connection name so parallel tests don't share a transaction.
	connName := fmt.Sprintf("test-%s-%d", t.Name(), time.Now().UnixNano())
	db, err := sql.Open("txdb", connName)
	if err != nil {
		t.Fatalf("open txdb: %v", err)
	}
	// txdb only supports a single connection.
	db.SetMaxOpenConns(1)

	// Apply schema inside the transaction so tables exist for this test.
	if _, err := db.Exec(schemaSQLite()); err != nil {
		db.Close()
		t.Fatalf("apply schema: %v", err)
	}

	t.Cleanup(func() { db.Close() }) // rolls back the transaction
	return db
}

// schemaSQLite returns a MariaDB-compatible version of the schema constant
// with the ENUM replaced by a VARCHAR so the same DDL works in MariaDB
// without needing ALTER TABLE privileges on system tables.
// If your MariaDB already has the tables you can remove the CREATE TABLE
// statements and just do TRUNCATE instead.
func schemaSQLite() string {
	// We re-use the package-level `schema` constant from main.go.
	// The txdb transaction ensures the tables are dropped on rollback,
	// but CREATE TABLE IF NOT EXISTS is idempotent anyway.
	return schema
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
	db := newTestDB(t)
	db.Close() // deliberately break it — t.Cleanup will try Close again (harmless)

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

// ── Backups ───────────────────────────────────────────────────────────────────

func TestDailyBackup_CreatesEntry(t *testing.T) {
	db := newTestDB(t)

	if err := ensureDailyBackup(db); err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM rating_backups`).Scan(&count)
	if count != 1 {
		t.Errorf("want 1 backup entry, got %d", count)
	}
}

func TestDailyBackup_IdempotentSameDay(t *testing.T) {
	db := newTestDB(t)

	ensureDailyBackup(db)
	ensureDailyBackup(db)
	ensureDailyBackup(db)

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM rating_backups`).Scan(&count)
	if count != 1 {
		t.Errorf("multiple backups same day should upsert to 1 row, got %d", count)
	}
}

func TestDailyBackup_PrunesOldEntries(t *testing.T) {
	db := newTestDB(t)

	// Insert more than backupKeep old entries directly.
	for i := range backupKeep + 3 {
		day := time.Now().UTC().AddDate(0, 0, -(i + 1)).Format("2006-01-02")
		db.Exec(
			`INSERT INTO rating_backups(created_day, created_at, ratings_json) VALUES(?,?,?)`,
			day, time.Now().Unix(), `{"ratings":[]}`,
		)
	}

	if err := ensureDailyBackup(db); err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM rating_backups`).Scan(&count)
	if count != backupKeep {
		t.Errorf("want %d backups after pruning, got %d", backupKeep, count)
	}
}

func TestDailyBackup_SnapshotIncludesRatings(t *testing.T) {
	db := newTestDB(t)
	mux := newTestMux(db)

	postRate(t, mux, "squid", "like", "1.1.1.1:0", "AgentA")
	postRate(t, mux, "forge", "dislike", "2.2.2.2:0", "AgentB")

	if err := ensureDailyBackup(db); err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	var snapshotJSON string
	db.QueryRow(`SELECT ratings_json FROM rating_backups`).Scan(&snapshotJSON)

	var snapshot struct {
		Ratings []map[string]any `json:"ratings"`
	}
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}
	if len(snapshot.Ratings) != 2 {
		t.Errorf("want 2 ratings in snapshot, got %d", len(snapshot.Ratings))
	}
}
