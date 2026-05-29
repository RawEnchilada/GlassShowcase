#define _POSIX_C_SOURCE 200809L

#include <arpa/inet.h>
#include <ctype.h>
#include <errno.h>
#include <fcntl.h>
#include <netinet/in.h>
#include <signal.h>
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <strings.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <time.h>
#include <unistd.h>

#include <sqlite3.h>

#define STATIC_DIR "public"
#define DB_PATH "data/ratings.sqlite3"
#define MAX_REQUEST 65536
#define MAX_BODY 4096
#define BACKUP_KEEP 5
#define RATE_LIMIT_BUCKETS 256
#define RATE_LIMIT_WINDOW_SECONDS 60
#define RATE_LIMIT_POSTS 30
#define str_value(x) #x
#define xstr(x) str_value(x)

static const char *PROJECT_IDS[] = {
    "blackwood",
    "conways",
    "crimson",
    "dirty",
    "forge",
    "game426",
    "nexus2",
    "orcwave",
    "sandstorm",
    "shipbattler",
    "shrimp",
    "squid",
    "ttl",
};

typedef struct {
    char ip[INET_ADDRSTRLEN];
    time_t window_start;
    int count;
} rate_bucket;

static rate_bucket rate_limits[RATE_LIMIT_BUCKETS];

static bool is_project_id(const char *id) {
    size_t count = sizeof(PROJECT_IDS) / sizeof(PROJECT_IDS[0]);
    for (size_t i = 0; i < count; i++) {
        if (strcmp(id, PROJECT_IDS[i]) == 0) {
            return true;
        }
    }
    return false;
}

static bool allow_rate_request(const char *ip, time_t now) {
    rate_bucket *slot = NULL;
    rate_bucket *oldest = &rate_limits[0];

    for (size_t i = 0; i < RATE_LIMIT_BUCKETS; i++) {
        if (rate_limits[i].ip[0] == '\0') {
            slot = &rate_limits[i];
            break;
        }
        if (strcmp(rate_limits[i].ip, ip) == 0) {
            slot = &rate_limits[i];
            break;
        }
        if (rate_limits[i].window_start < oldest->window_start) {
            oldest = &rate_limits[i];
        }
    }

    if (!slot) {
        slot = oldest;
        slot->ip[0] = '\0';
    }
    if (slot->ip[0] == '\0') {
        snprintf(slot->ip, sizeof(slot->ip), "%s", ip);
        slot->window_start = now;
        slot->count = 0;
    }
    if (now - slot->window_start >= RATE_LIMIT_WINDOW_SECONDS) {
        slot->window_start = now;
        slot->count = 0;
    }
    if (slot->count >= RATE_LIMIT_POSTS) {
        return false;
    }
    slot->count++;
    return true;
}

typedef struct {
    char *data;
    size_t len;
    size_t cap;
} string_buf;

static void buf_free(string_buf *buf) {
    free(buf->data);
    buf->data = NULL;
    buf->len = 0;
    buf->cap = 0;
}

static int buf_reserve(string_buf *buf, size_t extra) {
    if (extra > SIZE_MAX - buf->len - 1) {
        return -1;
    }
    size_t need = buf->len + extra + 1;
    if (need <= buf->cap) {
        return 0;
    }
    size_t cap = buf->cap ? buf->cap : 256;
    while (cap < need) {
        if (cap > SIZE_MAX / 2) {
            return -1;
        }
        cap *= 2;
    }
    char *data = realloc(buf->data, cap);
    if (!data) {
        return -1;
    }
    buf->data = data;
    buf->cap = cap;
    return 0;
}

static int buf_append(string_buf *buf, const char *text) {
    size_t len = strlen(text);
    if (buf_reserve(buf, len) != 0) {
        return -1;
    }
    memcpy(buf->data + buf->len, text, len);
    buf->len += len;
    buf->data[buf->len] = '\0';
    return 0;
}

static int buf_append_json_string(string_buf *buf, const char *text) {
    static const char hex[] = "0123456789abcdef";
    if (buf_append(buf, "\"") != 0) {
        return -1;
    }
    for (const unsigned char *p = (const unsigned char *)text; *p; p++) {
        if (*p == '"' || *p == '\\') {
            char escaped[3] = {'\\', (char)*p, '\0'};
            if (buf_append(buf, escaped) != 0) {
                return -1;
            }
        } else if (*p < 32) {
            char escaped[7] = {'\\', 'u', '0', '0', hex[*p >> 4], hex[*p & 0x0f], '\0'};
            if (buf_append(buf, escaped) != 0) {
                return -1;
            }
        } else {
            if (buf_reserve(buf, 1) != 0) {
                return -1;
            }
            buf->data[buf->len++] = (char)*p;
            buf->data[buf->len] = '\0';
        }
    }
    return buf_append(buf, "\"");
}

static int apply_schema(sqlite3 *db) {
    char *err = NULL;
    const char *schema =
        "PRAGMA journal_mode=WAL;"
        "PRAGMA synchronous=NORMAL;"
        "PRAGMA foreign_keys=ON;"
        "PRAGMA busy_timeout=2500;"
        "CREATE TABLE IF NOT EXISTS ratings ("
        "project_id TEXT NOT NULL,"
        "user_id TEXT NOT NULL,"
        "rating TEXT NOT NULL CHECK (rating IN ('like','dislike')),"
        "created_at INTEGER NOT NULL,"
        "PRIMARY KEY (project_id, user_id)"
        ");"
        "CREATE INDEX IF NOT EXISTS idx_ratings_project ON ratings(project_id);"
        "CREATE TABLE IF NOT EXISTS rating_backups ("
        "created_day TEXT PRIMARY KEY,"
        "created_at INTEGER NOT NULL,"
        "ratings_json TEXT NOT NULL"
        ");";

    if (sqlite3_exec(db, schema, NULL, NULL, &err) != SQLITE_OK) {
        fprintf(stderr, "sqlite schema failed: %s\n", err ? err : "unknown error");
        sqlite3_free(err);
        return -1;
    }

    return 0;
}

static void utc_day(time_t now, char out[11]) {
    struct tm tm;
    gmtime_r(&now, &tm);
    strftime(out, 11, "%Y-%m-%d", &tm);
}

static int build_ratings_snapshot(sqlite3 *db, char **out) {
    sqlite3_stmt *stmt = NULL;
    const char *sql =
        "SELECT project_id, user_id, rating, created_at "
        "FROM ratings ORDER BY project_id, user_id;";
    string_buf buf = {0};
    bool first = true;

    if (sqlite3_prepare_v2(db, sql, -1, &stmt, NULL) != SQLITE_OK) {
        return -1;
    }
    if (buf_append(&buf, "{\"ratings\":[") != 0) {
        sqlite3_finalize(stmt);
        buf_free(&buf);
        return -1;
    }

    while (sqlite3_step(stmt) == SQLITE_ROW) {
        const char *project_id = (const char *)sqlite3_column_text(stmt, 0);
        const char *user_id = (const char *)sqlite3_column_text(stmt, 1);
        const char *rating = (const char *)sqlite3_column_text(stmt, 2);
        sqlite3_int64 created_at = sqlite3_column_int64(stmt, 3);
        char created[64];

        snprintf(created, sizeof(created), "%lld", (long long)created_at);
        if ((!first && buf_append(&buf, ",") != 0) ||
            buf_append(&buf, "{\"projectId\":") != 0 ||
            buf_append_json_string(&buf, project_id ? project_id : "") != 0 ||
            buf_append(&buf, ",\"userId\":") != 0 ||
            buf_append_json_string(&buf, user_id ? user_id : "") != 0 ||
            buf_append(&buf, ",\"rating\":") != 0 ||
            buf_append_json_string(&buf, rating ? rating : "") != 0 ||
            buf_append(&buf, ",\"createdAt\":") != 0 ||
            buf_append(&buf, created) != 0 ||
            buf_append(&buf, "}") != 0) {
            sqlite3_finalize(stmt);
            buf_free(&buf);
            return -1;
        }
        first = false;
    }

    if (sqlite3_finalize(stmt) != SQLITE_OK || buf_append(&buf, "]}") != 0) {
        buf_free(&buf);
        return -1;
    }
    *out = buf.data;
    return 0;
}

static int ensure_daily_backup(sqlite3 *db) {
    sqlite3_stmt *stmt = NULL;
    time_t now = time(NULL);
    char day[11];
    char *snapshot = NULL;
    int rc = -1;

    utc_day(now, day);

    if (sqlite3_exec(db, "BEGIN IMMEDIATE;", NULL, NULL, NULL) != SQLITE_OK) {
        return -1;
    }

    if (build_ratings_snapshot(db, &snapshot) != 0) {
        goto done;
    }

    if (sqlite3_prepare_v2(
            db,
            "INSERT INTO rating_backups(created_day, created_at, ratings_json) VALUES(?,?,?) "
            "ON CONFLICT(created_day) DO UPDATE SET "
            "created_at=excluded.created_at,"
            "ratings_json=excluded.ratings_json;",
            -1,
            &stmt,
            NULL
        ) != SQLITE_OK) {
        goto done;
    }
    sqlite3_bind_text(stmt, 1, day, -1, SQLITE_STATIC);
    sqlite3_bind_int64(stmt, 2, (sqlite3_int64)now);
    sqlite3_bind_text(stmt, 3, snapshot, -1, SQLITE_TRANSIENT);
    if (sqlite3_step(stmt) != SQLITE_DONE) {
        goto done;
    }
    sqlite3_finalize(stmt);
    stmt = NULL;

    if (sqlite3_exec(
            db,
            "DELETE FROM rating_backups "
            "WHERE created_day NOT IN ("
            "SELECT created_day FROM rating_backups ORDER BY created_day DESC LIMIT " xstr(BACKUP_KEEP) ");",
            NULL,
            NULL,
            NULL
        ) != SQLITE_OK) {
        goto done;
    }

    rc = 0;

done:
    if (stmt) {
        sqlite3_finalize(stmt);
    }
    free(snapshot);
    sqlite3_exec(db, rc == 0 ? "COMMIT;" : "ROLLBACK;", NULL, NULL, NULL);
    return rc;
}

static int ensure_data_dir(void) {
    struct stat st;

    if (stat("data", &st) == 0) {
        if (S_ISDIR(st.st_mode)) {
            return 0;
        }
        fprintf(stderr, "data exists but is not a directory\n");
        return -1;
    }
    if (errno != ENOENT) {
        perror("stat data");
        return -1;
    }
    if (mkdir("data", 0750) != 0 && errno != EEXIST) {
        perror("mkdir data");
        return -1;
    }
    return 0;
}

static int init_db(sqlite3 **db) {
    if (ensure_data_dir() != 0) {
        return -1;
    }
    if (sqlite3_open_v2(DB_PATH, db, SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE | SQLITE_OPEN_FULLMUTEX, NULL) != SQLITE_OK) {
        fprintf(stderr, "sqlite open failed: %s\n", sqlite3_errmsg(*db));
        return -1;
    }

    if (apply_schema(*db) != 0) {
        return -1;
    }
    if (ensure_daily_backup(*db) != 0) {
        fprintf(stderr, "sqlite backup failed: %s\n", sqlite3_errmsg(*db));
        return -1;
    }
    return 0;
}

static bool valid_user_id(const char *id) {
    if (!id || strlen(id) != 32) {
        return false;
    }
    for (size_t i = 0; i < 32; i++) {
        if (!isxdigit((unsigned char)id[i])) {
            return false;
        }
    }
    return true;
}

static void random_user_id(char out[33]) {
    static const char hex[] = "0123456789abcdef";
    unsigned char bytes[16];
    int fd = open("/dev/urandom", O_RDONLY);
    if (fd >= 0) {
        ssize_t got = read(fd, bytes, sizeof(bytes));
        close(fd);
        if (got == (ssize_t)sizeof(bytes)) {
            for (size_t i = 0; i < sizeof(bytes); i++) {
                out[i * 2] = hex[bytes[i] >> 4];
                out[i * 2 + 1] = hex[bytes[i] & 0x0f];
            }
            out[32] = '\0';
            return;
        }
    }

    srand((unsigned int)(time(NULL) ^ getpid()));
    for (size_t i = 0; i < 32; i++) {
        out[i] = hex[rand() & 0x0f];
    }
    out[32] = '\0';
}

static bool cookie_value(const char *cookies, const char *name, char *out, size_t out_size) {
    if (!cookies || !name || out_size == 0) {
        return false;
    }

    size_t name_len = strlen(name);
    const char *p = cookies;
    while (*p) {
        while (*p == ' ' || *p == ';') {
            p++;
        }
        if (strncmp(p, name, name_len) == 0 && p[name_len] == '=') {
            p += name_len + 1;
            size_t i = 0;
            while (*p && *p != ';' && i + 1 < out_size) {
                out[i++] = *p++;
            }
            out[i] = '\0';
            return true;
        }
        while (*p && *p != ';') {
            p++;
        }
    }
    return false;
}

static void ensure_user(const char *cookies, char user_id[33], bool *set_cookie) {
    char candidate[64] = {0};
    if (cookie_value(cookies, "gt_user", candidate, sizeof(candidate)) && valid_user_id(candidate)) {
        strcpy(user_id, candidate);
        *set_cookie = false;
        return;
    }
    random_user_id(user_id);
    *set_cookie = true;
}

static const char *reason_phrase(int status) {
    switch (status) {
        case 200: return "OK";
        case 201: return "Created";
        case 400: return "Bad Request";
        case 404: return "Not Found";
        case 405: return "Method Not Allowed";
        case 409: return "Conflict";
        case 413: return "Payload Too Large";
        case 429: return "Too Many Requests";
        default: return "Internal Server Error";
    }
}

static int send_all(int fd, const void *buf, size_t len) {
    const char *p = (const char *)buf;
    while (len > 0) {
        ssize_t sent = send(fd, p, len, 0);
        if (sent <= 0) {
            if (errno == EINTR) {
                continue;
            }
            return -1;
        }
        p += sent;
        len -= (size_t)sent;
    }
    return 0;
}

static void send_response(int fd, int status, const char *type, const char *body, size_t len, const char *user_id, bool set_cookie) {
    char header[1024];
    int n = snprintf(
        header,
        sizeof(header),
        "HTTP/1.1 %d %s\r\n"
        "Content-Type: %s\r\n"
        "Content-Length: %zu\r\n"
        "Cache-Control: no-store\r\n"
        "X-Content-Type-Options: nosniff\r\n"
        "Connection: close\r\n",
        status,
        reason_phrase(status),
        type,
        len
    );
    send_all(fd, header, (size_t)n);
    if (set_cookie && user_id) {
        char cookie[256];
        n = snprintf(
            cookie,
            sizeof(cookie),
            "Set-Cookie: gt_user=%s; Max-Age=31536000; Path=/; SameSite=Lax; HttpOnly\r\n",
            user_id
        );
        send_all(fd, cookie, (size_t)n);
    }
    send_all(fd, "\r\n", 2);
    if (len > 0) {
        send_all(fd, body, len);
    }
}

static void send_json_text(int fd, int status, const char *json, const char *user_id, bool set_cookie) {
    send_response(fd, status, "application/json; charset=utf-8", json, strlen(json), user_id, set_cookie);
}

static const char *content_type(const char *path) {
    const char *ext = strrchr(path, '.');
    if (!ext) return "application/octet-stream";
    if (strcmp(ext, ".html") == 0) return "text/html; charset=utf-8";
    if (strcmp(ext, ".css") == 0) return "text/css; charset=utf-8";
    if (strcmp(ext, ".js") == 0) return "text/javascript; charset=utf-8";
    if (strcmp(ext, ".png") == 0) return "image/png";
    if (strcmp(ext, ".webp") == 0) return "image/webp";
    if (strcmp(ext, ".jpg") == 0 || strcmp(ext, ".jpeg") == 0) return "image/jpeg";
    if (strcmp(ext, ".svg") == 0) return "image/svg+xml";
    return "application/octet-stream";
}

static bool safe_path(const char *url_path) {
    return url_path[0] == '/' && strstr(url_path, "..") == NULL && strchr(url_path, '\\') == NULL;
}

static int hex_value(char c) {
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    return -1;
}

static bool url_decode_path(const char *src, char *dst, size_t dst_size) {
    size_t out = 0;
    for (size_t i = 0; src[i]; i++) {
        if (out + 1 >= dst_size) {
            return false;
        }
        if (src[i] == '%' && src[i + 1] && src[i + 2]) {
            int hi = hex_value(src[i + 1]);
            int lo = hex_value(src[i + 2]);
            if (hi < 0 || lo < 0) {
                return false;
            }
            dst[out++] = (char)((hi << 4) | lo);
            i += 2;
        } else {
            dst[out++] = src[i];
        }
    }
    dst[out] = '\0';
    return true;
}

static void serve_file(int fd, const char *url_path, const char *user_id, bool set_cookie) {
    char decoded_path[512];
    if (!url_decode_path(url_path, decoded_path, sizeof(decoded_path)) || !safe_path(decoded_path)) {
        send_json_text(fd, 400, "{\"error\":\"bad path\"}", user_id, set_cookie);
        return;
    }

    char file_path[1024];
    if (strcmp(decoded_path, "/") == 0) {
        snprintf(file_path, sizeof(file_path), "%s/index.html", STATIC_DIR);
    } else {
        snprintf(file_path, sizeof(file_path), "%s%s", STATIC_DIR, decoded_path);
    }

    int file = open(file_path, O_RDONLY);
    if (file < 0) {
        send_json_text(fd, 404, "{\"error\":\"not found\"}", user_id, set_cookie);
        return;
    }

    struct stat st;
    if (fstat(file, &st) != 0 || !S_ISREG(st.st_mode)) {
        close(file);
        send_json_text(fd, 404, "{\"error\":\"not found\"}", user_id, set_cookie);
        return;
    }

    char header[1024];
    int n = snprintf(
        header,
        sizeof(header),
        "HTTP/1.1 200 OK\r\n"
        "Content-Type: %s\r\n"
        "Content-Length: %lld\r\n"
        "Cache-Control: public, max-age=3600\r\n"
        "X-Content-Type-Options: nosniff\r\n"
        "Connection: close\r\n",
        content_type(file_path),
        (long long)st.st_size
    );
    send_all(fd, header, (size_t)n);
    if (set_cookie && user_id) {
        char cookie[256];
        n = snprintf(cookie, sizeof(cookie), "Set-Cookie: gt_user=%s; Max-Age=31536000; Path=/; SameSite=Lax; HttpOnly\r\n", user_id);
        send_all(fd, cookie, (size_t)n);
    }
    send_all(fd, "\r\n", 2);

    char buf[16384];
    ssize_t got;
    while ((got = read(file, buf, sizeof(buf))) > 0) {
        if (send_all(fd, buf, (size_t)got) != 0) {
            break;
        }
    }
    close(file);
}

static int rating_counts(sqlite3 *db, const char *project_id, const char *user_id, int *likes, int *dislikes, char *mine, size_t mine_size) {
    sqlite3_stmt *stmt = NULL;
    *likes = 0;
    *dislikes = 0;
    mine[0] = '\0';

    const char *count_sql =
        "SELECT "
        "SUM(CASE WHEN rating='like' THEN 1 ELSE 0 END),"
        "SUM(CASE WHEN rating='dislike' THEN 1 ELSE 0 END) "
        "FROM ratings WHERE project_id=?;";
    if (sqlite3_prepare_v2(db, count_sql, -1, &stmt, NULL) != SQLITE_OK) {
        return -1;
    }
    sqlite3_bind_text(stmt, 1, project_id, -1, SQLITE_STATIC);
    if (sqlite3_step(stmt) == SQLITE_ROW) {
        *likes = sqlite3_column_int(stmt, 0);
        *dislikes = sqlite3_column_int(stmt, 1);
    }
    sqlite3_finalize(stmt);

    const char *mine_sql = "SELECT rating FROM ratings WHERE project_id=? AND user_id=?;";
    if (sqlite3_prepare_v2(db, mine_sql, -1, &stmt, NULL) != SQLITE_OK) {
        return -1;
    }
    sqlite3_bind_text(stmt, 1, project_id, -1, SQLITE_STATIC);
    sqlite3_bind_text(stmt, 2, user_id, -1, SQLITE_STATIC);
    if (sqlite3_step(stmt) == SQLITE_ROW) {
        snprintf(mine, mine_size, "%s", (const char *)sqlite3_column_text(stmt, 0));
    }
    sqlite3_finalize(stmt);
    return 0;
}

static bool db_healthy(sqlite3 *db) {
    sqlite3_stmt *stmt = NULL;
    bool healthy = false;

    if (sqlite3_prepare_v2(db, "SELECT 1;", -1, &stmt, NULL) != SQLITE_OK) {
        return false;
    }
    healthy = sqlite3_step(stmt) == SQLITE_ROW;
    sqlite3_finalize(stmt);
    return healthy;
}

static void healthcheck(int fd, sqlite3 *db) {
    if (!db_healthy(db)) {
        send_json_text(fd, 500, "{\"ok\":false,\"error\":\"database\"}", NULL, false);
        return;
    }
    send_json_text(fd, 200, "{\"ok\":true}", NULL, false);
}

static void api_ratings(int fd, sqlite3 *db, const char *user_id, bool set_cookie) {
    char json[4096];
    size_t used = 0;
    used += (size_t)snprintf(json + used, sizeof(json) - used, "{\"ratings\":{");

    size_t count = sizeof(PROJECT_IDS) / sizeof(PROJECT_IDS[0]);
    for (size_t i = 0; i < count; i++) {
        int likes = 0;
        int dislikes = 0;
        char mine[16];
        if (rating_counts(db, PROJECT_IDS[i], user_id, &likes, &dislikes, mine, sizeof(mine)) != 0) {
            send_json_text(fd, 500, "{\"error\":\"database error\"}", user_id, set_cookie);
            return;
        }
        used += (size_t)snprintf(
            json + used,
            sizeof(json) - used,
            "%s\"%s\":{\"likes\":%d,\"dislikes\":%d,\"mine\":\"%s\"}",
            i ? "," : "",
            PROJECT_IDS[i],
            likes,
            dislikes,
            mine
        );
    }
    snprintf(json + used, sizeof(json) - used, "}}");
    send_json_text(fd, 200, json, user_id, set_cookie);
}

static bool json_field(const char *body, const char *name, char *out, size_t out_size) {
    char needle[64];
    snprintf(needle, sizeof(needle), "\"%s\"", name);
    const char *p = strstr(body, needle);
    if (!p) return false;
    p = strchr(p + strlen(needle), ':');
    if (!p) return false;
    p++;
    while (isspace((unsigned char)*p)) p++;
    if (*p != '"') return false;
    p++;
    size_t i = 0;
    while (*p && *p != '"' && i + 1 < out_size) {
        if ((unsigned char)*p < 32 || *p == '\\') return false;
        out[i++] = *p++;
    }
    if (*p != '"') return false;
    out[i] = '\0';
    return true;
}

static void api_rate(int fd, sqlite3 *db, const char *body, const char *user_id, bool set_cookie) {
    char project_id[64];
    char rating[16];
    if (!json_field(body, "projectId", project_id, sizeof(project_id)) || !json_field(body, "rating", rating, sizeof(rating))) {
        send_json_text(fd, 400, "{\"error\":\"expected projectId and rating\"}", user_id, set_cookie);
        return;
    }
    if (!is_project_id(project_id) || (strcmp(rating, "like") != 0 && strcmp(rating, "dislike") != 0)) {
        send_json_text(fd, 400, "{\"error\":\"invalid rating\"}", user_id, set_cookie);
        return;
    }

    sqlite3_stmt *stmt = NULL;
    const char *sql = "INSERT OR IGNORE INTO ratings(project_id, user_id, rating, created_at) VALUES(?,?,?,?);";
    if (sqlite3_prepare_v2(db, sql, -1, &stmt, NULL) != SQLITE_OK) {
        send_json_text(fd, 500, "{\"error\":\"database error\"}", user_id, set_cookie);
        return;
    }
    sqlite3_bind_text(stmt, 1, project_id, -1, SQLITE_STATIC);
    sqlite3_bind_text(stmt, 2, user_id, -1, SQLITE_STATIC);
    sqlite3_bind_text(stmt, 3, rating, -1, SQLITE_STATIC);
    sqlite3_bind_int64(stmt, 4, (sqlite3_int64)time(NULL));

    int rc = sqlite3_step(stmt);
    int changed = sqlite3_changes(db);
    sqlite3_finalize(stmt);
    if (rc != SQLITE_DONE) {
        send_json_text(fd, 500, "{\"error\":\"database error\"}", user_id, set_cookie);
        return;
    }
    if (changed && ensure_daily_backup(db) != 0) {
        fprintf(stderr, "sqlite backup failed: %s\n", sqlite3_errmsg(db));
    }

    int likes = 0;
    int dislikes = 0;
    char mine[16];
    rating_counts(db, project_id, user_id, &likes, &dislikes, mine, sizeof(mine));

    char json[512];
    snprintf(
        json,
        sizeof(json),
        "{\"accepted\":%s,\"projectId\":\"%s\",\"likes\":%d,\"dislikes\":%d,\"mine\":\"%s\"}",
        changed ? "true" : "false",
        project_id,
        likes,
        dislikes,
        mine
    );
    send_json_text(fd, changed ? 201 : 409, json, user_id, set_cookie);
}

static const char *header_value(const char *headers, const char *name, char *out, size_t out_size) {
    size_t name_len = strlen(name);
    const char *p = headers;
    while ((p = strstr(p, name)) != NULL) {
        bool at_line = p == headers || p[-1] == '\n';
        if (at_line && strncasecmp(p, name, name_len) == 0 && p[name_len] == ':') {
            p += name_len + 1;
            while (*p == ' ' || *p == '\t') p++;
            size_t i = 0;
            while (*p && *p != '\r' && *p != '\n' && i + 1 < out_size) {
                out[i++] = *p++;
            }
            out[i] = '\0';
            return out;
        }
        p += name_len;
    }
    return NULL;
}

static void handle_client(int fd, sqlite3 *db, const char *client_ip) {
    char req[MAX_REQUEST + 1];
    size_t total = 0;
    ssize_t got;
    char *header_end = NULL;

    while (total < MAX_REQUEST) {
        got = recv(fd, req + total, MAX_REQUEST - total, 0);
        if (got <= 0) return;
        total += (size_t)got;
        req[total] = '\0';
        header_end = strstr(req, "\r\n\r\n");
        if (header_end) {
            break;
        }
    }
    if (!header_end) {
        send_json_text(fd, 400, "{\"error\":\"bad request\"}", NULL, false);
        return;
    }

    char method[8] = {0};
    char path[256] = {0};
    if (sscanf(req, "%7s %255s", method, path) != 2) {
        send_json_text(fd, 400, "{\"error\":\"bad request\"}", NULL, false);
        return;
    }

    char cookies[1024] = {0};
    char content_len_header[32] = {0};
    header_value(req, "Cookie", cookies, sizeof(cookies));
    header_value(req, "Content-Length", content_len_header, sizeof(content_len_header));

    int content_len = content_len_header[0] ? atoi(content_len_header) : 0;
    if (content_len < 0 || content_len > MAX_BODY) {
        send_json_text(fd, 413, "{\"error\":\"body too large\"}", NULL, false);
        return;
    }

    size_t header_bytes = (size_t)(header_end + 4 - req);
    while (total < header_bytes + (size_t)content_len && total < MAX_REQUEST) {
        got = recv(fd, req + total, MAX_REQUEST - total, 0);
        if (got <= 0) return;
        total += (size_t)got;
        req[total] = '\0';
    }

    char user_id[33];
    bool set_cookie = false;
    ensure_user(cookies, user_id, &set_cookie);

    char *query = strchr(path, '?');
    if (query) {
        *query = '\0';
    }

    if (strcmp(method, "GET") == 0 && strcmp(path, "/health") == 0) {
        healthcheck(fd, db);
        return;
    }

    if (strcmp(method, "GET") == 0 && strcmp(path, "/api/ratings") == 0) {
        api_ratings(fd, db, user_id, set_cookie);
        return;
    }

    if (strcmp(method, "POST") == 0 && strcmp(path, "/api/rate") == 0) {
        if (!allow_rate_request(client_ip, time(NULL))) {
            send_json_text(fd, 429, "{\"error\":\"rate limit exceeded\"}", user_id, set_cookie);
            return;
        }
        char *body = req + header_bytes;
        body[content_len] = '\0';
        api_rate(fd, db, body, user_id, set_cookie);
        return;
    }

    if (strcmp(method, "GET") == 0 || strcmp(method, "HEAD") == 0) {
        serve_file(fd, path, user_id, set_cookie);
        return;
    }

    send_json_text(fd, 405, "{\"error\":\"method not allowed\"}", user_id, set_cookie);
}

#ifndef GLASS_TOWER_TEST
int main(void) {
    signal(SIGPIPE, SIG_IGN);

    sqlite3 *db = NULL;
    if (init_db(&db) != 0) {
        return 1;
    }

    const char *port_env = getenv("PORT");
    int port = port_env ? atoi(port_env) : 8080;
    if (port <= 0 || port > 65535) {
        port = 8080;
    }

    int server = socket(AF_INET, SOCK_STREAM, 0);
    if (server < 0) {
        perror("socket");
        return 1;
    }

    int yes = 1;
    setsockopt(server, SOL_SOCKET, SO_REUSEADDR, &yes, sizeof(yes));

    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    addr.sin_addr.s_addr = htonl(INADDR_ANY);
    addr.sin_port = htons((uint16_t)port);

    if (bind(server, (struct sockaddr *)&addr, sizeof(addr)) != 0) {
        perror("bind");
        close(server);
        return 1;
    }

    if (listen(server, 64) != 0) {
        perror("listen");
        close(server);
        return 1;
    }

    printf("Glass Tower Archive listening on http://127.0.0.1:%d\n", port);
    fflush(stdout);

    for (;;) {
        struct sockaddr_in client_addr;
        socklen_t client_len = sizeof(client_addr);
        char client_ip[INET_ADDRSTRLEN] = "unknown";
        int client = accept(server, (struct sockaddr *)&client_addr, &client_len);
        if (client < 0) {
            if (errno == EINTR) {
                continue;
            }
            perror("accept");
            break;
        }
        inet_ntop(AF_INET, &client_addr.sin_addr, client_ip, sizeof(client_ip));
        handle_client(client, db, client_ip);
        close(client);
    }

    close(server);
    sqlite3_close(db);
    return 0;
}
#endif
