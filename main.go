package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// ── Config ────────────────────────────────────────────────────────────────────

const (
	staticDir       = "public"
	rateLimitWindow = 60 * time.Second
	rateLimitPosts  = 30
)

var projectIDs = map[string]bool{
	"blackwood":   true,
	"conways":     true,
	"crimson":     true,
	"dirty":       true,
	"forge":       true,
	"game426":     true,
	"nexus2":      true,
	"orcwave":     true,
	"sandstorm":   true,
	"shipbattler": true,
	"shrimp":      true,
	"squid":       true,
	"ttl":         true,
}

// ── Rate limiter ──────────────────────────────────────────────────────────────

type rateBucket struct {
	windowStart time.Time
	count       int
}

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{buckets: make(map[string]*rateBucket)}
}

func (r *rateLimiter) allow(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	b, ok := r.buckets[ip]
	if !ok {
		r.buckets[ip] = &rateBucket{windowStart: now, count: 1}
		return true
	}
	if now.Sub(b.windowStart) >= rateLimitWindow {
		b.windowStart = now
		b.count = 0
	}
	if b.count >= rateLimitPosts {
		return false
	}
	b.count++
	return true
}

// ── Database ──────────────────────────────────────────────────────────────────

const schema = `
CREATE TABLE IF NOT EXISTS ratings (
    project_id VARCHAR(64) NOT NULL,
    user_id    VARCHAR(64) NOT NULL,
    rating     ENUM('like','dislike') NOT NULL,
    created_at BIGINT NOT NULL,
    PRIMARY KEY (project_id, user_id),
    INDEX idx_ratings_project (project_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`

func initDB() (*sql.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is not set")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Connection pool tuning for network environments
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Minute * 3)

	statements := strings.Split(schema, ";")
	for _, stmt := range statements {
		trimmed := strings.TrimSpace(stmt)
		if trimmed == "" {
			continue
		}
		if _, err := db.Exec(trimmed); err != nil {
			return nil, fmt.Errorf("apply schema section: %w", err)
		}
	}

	return db, nil
}

// -- User --------------------------

func getUserId(addr string, agent string) string {
	ip := addr
	if host, _, err := net.SplitHostPort(addr); err == nil {
		ip = host
	}
	h := sha256.Sum256([]byte(ip + strings.ReplaceAll(agent, " ", "")))
	return hex.EncodeToString(h[:])
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func handleHealth(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "database"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

type ratingCounts struct {
	Likes    int    `json:"likes"`
	Dislikes int    `json:"dislikes"`
	Mine     string `json:"mine"`
}

func getRatingCounts(db *sql.DB, projectID, userID string) (ratingCounts, error) {
	var rc ratingCounts
	var likes, dislikes sql.NullInt64

	err := db.QueryRow(`
        SELECT
            SUM(CASE WHEN rating='like'    THEN 1 ELSE 0 END),
            SUM(CASE WHEN rating='dislike' THEN 1 ELSE 0 END)
        FROM ratings WHERE project_id = ?`, projectID,
	).Scan(&likes, &dislikes)
	if err != nil && err != sql.ErrNoRows {
		return rc, err
	}
	rc.Likes = int(likes.Int64)
	rc.Dislikes = int(dislikes.Int64)

	row := db.QueryRow(
		`SELECT rating FROM ratings WHERE project_id = ? AND user_id = ?`,
		projectID, userID,
	)
	row.Scan(&rc.Mine)
	return rc, nil
}

func handleGetRatings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := getUserId(r.RemoteAddr, r.UserAgent())
		result := make(map[string]ratingCounts, len(projectIDs))
		for id := range projectIDs {
			rc, err := getRatingCounts(db, id, uid)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "database error"})
				return
			}
			result[id] = rc
		}
		writeJSON(w, http.StatusOK, map[string]any{"ratings": result})
	}
}

func handlePostRate(db *sql.DB, limiter *rateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := getUserId(r.RemoteAddr, r.UserAgent())

		ip := r.RemoteAddr
		if !limiter.allow(ip) {
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate limit exceeded"})
			return
		}

		var body struct {
			ProjectID string `json:"projectId"`
			Rating    string `json:"rating"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "expected projectId and rating"})
			return
		}
		if !projectIDs[body.ProjectID] || (body.Rating != "like" && body.Rating != "dislike") {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid rating"})
			return
		}

		println(uid + " posted a rating for " + body.ProjectID + " - " + body.Rating)

		res, err := db.Exec(
			`INSERT IGNORE INTO ratings(project_id, user_id, rating, created_at) VALUES(?,?,?,?)`,
			body.ProjectID, uid, body.Rating, time.Now().Unix(),
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "database error"})
			return
		}

		changed, _ := res.RowsAffected()

		rc, err := getRatingCounts(db, body.ProjectID, uid)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "database error"})
			return
		}

		status := http.StatusConflict
		if changed > 0 {
			status = http.StatusCreated
		}
		writeJSON(w, status, map[string]any{
			"accepted":  changed > 0,
			"projectId": body.ProjectID,
			"likes":     rc.Likes,
			"dislikes":  rc.Dislikes,
			"mine":      rc.Mine,
		})
	}
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	db, err := initDB()
	if err != nil {
		log.Fatalf("init db: %v", err)
	}
	defer db.Close()

	limiter := newRateLimiter()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth(db))
	mux.HandleFunc("GET /api/ratings", handleGetRatings(db))
	mux.HandleFunc("POST /api/rate", handlePostRate(db, limiter))
	mux.Handle("/", http.FileServer(http.Dir(staticDir)))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Glass Tower Archive listening on http://127.0.0.1:%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}
