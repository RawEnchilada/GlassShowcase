#define GLASS_TOWER_TEST
#if defined(__GNUC__)
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wunused-function"
#endif
#include "server.c"
#if defined(__GNUC__)
#pragma GCC diagnostic pop
#endif

#include <assert.h>

static sqlite3 *test_db(void) {
    sqlite3 *db = NULL;
    assert(sqlite3_open(":memory:", &db) == SQLITE_OK);
    assert(apply_schema(db) == 0);
    return db;
}

static void insert_rating(sqlite3 *db, const char *project_id, const char *user_id, const char *rating) {
    sqlite3_stmt *stmt = NULL;
    const char *sql = "INSERT OR IGNORE INTO ratings(project_id, user_id, rating, created_at) VALUES(?,?,?,123);";
    assert(sqlite3_prepare_v2(db, sql, -1, &stmt, NULL) == SQLITE_OK);
    sqlite3_bind_text(stmt, 1, project_id, -1, SQLITE_STATIC);
    sqlite3_bind_text(stmt, 2, user_id, -1, SQLITE_STATIC);
    sqlite3_bind_text(stmt, 3, rating, -1, SQLITE_STATIC);
    assert(sqlite3_step(stmt) == SQLITE_DONE);
    sqlite3_finalize(stmt);
}

static void test_project_validation(void) {
    assert(is_project_id("blackwood"));
    assert(is_project_id("conways"));
    assert(is_project_id("crimson"));
    assert(is_project_id("dirty"));
    assert(is_project_id("game426"));
    assert(is_project_id("nexus2"));
    assert(is_project_id("orcwave"));
    assert(is_project_id("sandstorm"));
    assert(is_project_id("shipbattler"));
    assert(is_project_id("shrimp"));
    assert(is_project_id("squid"));
    assert(is_project_id("ttl"));
    assert(is_project_id("forge"));
    assert(!is_project_id("../data/ratings.sqlite3"));
    assert(!is_project_id("unknown"));
}

static void test_user_id_validation(void) {
    assert(valid_user_id("0123456789abcdef0123456789abcdef"));
    assert(!valid_user_id("0123456789abcdef0123456789abcdeg"));
    assert(!valid_user_id("short"));
    assert(!valid_user_id(NULL));
}

static void test_cookie_parsing(void) {
    char out[64] = {0};
    assert(cookie_value("theme=dark; gt_user=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa; other=1", "gt_user", out, sizeof(out)));
    assert(strcmp(out, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") == 0);
    assert(!cookie_value("theme=dark; other=1", "gt_user", out, sizeof(out)));
}

static void test_json_field(void) {
    char out[64] = {0};
    assert(json_field("{\"projectId\":\"moth-signal\",\"rating\":\"like\"}", "projectId", out, sizeof(out)));
    assert(strcmp(out, "moth-signal") == 0);
    assert(json_field("{\"projectId\":\"moth-signal\",\"rating\":\"like\"}", "rating", out, sizeof(out)));
    assert(strcmp(out, "like") == 0);
    assert(!json_field("{\"rating\":\"dis\\nlike\"}", "rating", out, sizeof(out)));
}

static void test_url_decode_path(void) {
    char out[128] = {0};
    assert(url_decode_path("/assets/ttl/2026-05-29%2015-18-08.gif", out, sizeof(out)));
    assert(strcmp(out, "/assets/ttl/2026-05-29 15-18-08.gif") == 0);
    assert(!url_decode_path("/bad/%xx", out, sizeof(out)));
}

static void test_rating_counts_and_duplicate_lock(void) {
    sqlite3 *db = test_db();
    int likes = 0;
    int dislikes = 0;
    char mine[16] = {0};

    insert_rating(db, "dirty", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "like");
    insert_rating(db, "dirty", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "dislike");
    insert_rating(db, "dirty", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "dislike");

    assert(rating_counts(db, "dirty", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", &likes, &dislikes, mine, sizeof(mine)) == 0);
    assert(likes == 1);
    assert(dislikes == 1);
    assert(strcmp(mine, "like") == 0);

    sqlite3_close(db);
}

static void test_daily_backup_refresh_and_prune(void) {
    sqlite3 *db = test_db();
    sqlite3_stmt *stmt = NULL;
    int count = 0;
    char day[11];

    insert_rating(db, "dirty", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "like");
    assert(ensure_daily_backup(db) == 0);
    utc_day(time(NULL), day);

    assert(sqlite3_prepare_v2(db, "SELECT ratings_json FROM rating_backups WHERE created_day=?;", -1, &stmt, NULL) == SQLITE_OK);
    sqlite3_bind_text(stmt, 1, day, -1, SQLITE_STATIC);
    assert(sqlite3_step(stmt) == SQLITE_ROW);
    assert(strstr((const char *)sqlite3_column_text(stmt, 0), "\"projectId\":\"dirty\"") != NULL);
    assert(strstr((const char *)sqlite3_column_text(stmt, 0), "\"rating\":\"like\"") != NULL);
    sqlite3_finalize(stmt);

    assert(sqlite3_exec(
        db,
        "INSERT OR REPLACE INTO rating_backups(created_day, created_at, ratings_json) VALUES"
        "('2000-01-01',1,'{}'),"
        "('2000-01-02',2,'{}'),"
        "('2000-01-03',3,'{}'),"
        "('2000-01-04',4,'{}'),"
        "('2000-01-05',5,'{}'),"
        "('2000-01-06',6,'{}');",
        NULL,
        NULL,
        NULL
    ) == SQLITE_OK);
    assert(ensure_daily_backup(db) == 0);

    assert(sqlite3_prepare_v2(db, "SELECT COUNT(*) FROM rating_backups;", -1, &stmt, NULL) == SQLITE_OK);
    assert(sqlite3_step(stmt) == SQLITE_ROW);
    count = sqlite3_column_int(stmt, 0);
    sqlite3_finalize(stmt);
    assert(count == 5);

    sqlite3_close(db);
}

static void test_rate_limit(void) {
    const char *ip = "203.0.113.10";
    time_t now = 1000;

    for (int i = 0; i < RATE_LIMIT_POSTS; i++) {
        assert(allow_rate_request(ip, now));
    }
    assert(!allow_rate_request(ip, now));
    assert(allow_rate_request(ip, now + RATE_LIMIT_WINDOW_SECONDS));
}

int main(void) {
    test_project_validation();
    test_user_id_validation();
    test_cookie_parsing();
    test_json_field();
    test_url_decode_path();
    test_rating_counts_and_duplicate_lock();
    test_daily_backup_refresh_and_prune();
    test_rate_limit();
    puts("All unit tests passed.");
    return 0;
}
