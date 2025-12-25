package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	DB *sql.DB
}

func Open(dbPath string) (*Store, error) {
	// ensure folder exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// basic pragmas
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA foreign_keys=ON;`); err != nil {
		_ = db.Close()
		return nil, err
	}

	s := &Store{DB: db}
	if err := s.Migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) Migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS images (
  id          TEXT PRIMARY KEY,
  path        TEXT NOT NULL,
  sha256      TEXT NOT NULL UNIQUE,
  fetched_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS labels (
  image_id    TEXT PRIMARY KEY,
  skystate     TEXT NOT NULL,      -- clear|light_clouds|heavy_clouds|precipitation|unknown
  meteor      INTEGER NOT NULL,   -- 0/1
  labeled_at  TEXT NOT NULL,
  FOREIGN KEY(image_id) REFERENCES images(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_images_fetched_at ON images(fetched_at);
`
	_, err := s.DB.Exec(schema)
	if err != nil {
		return err
	}

	// Backfill optional columns that may not exist in older databases.
	if err := ensureColumn(s.DB, "images", "size_bytes", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	return nil
}

// ensureColumn adds the column if it's missing (idempotent for repeated migrations).
func ensureColumn(db *sql.DB, table, column, columnDef string) error {
	var count int
	query := fmt.Sprintf(`SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = ?`, table)
	if err := db.QueryRow(query, column).Scan(&count); err != nil {
		return fmt.Errorf("check column %s.%s: %w", table, column, err)
	}
	if count > 0 {
		return nil
	}
	ddl := fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, columnDef)
	if _, err := db.Exec(ddl); err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, column, err)
	}
	return nil
}

type Image struct {
	ID        string
	Path      string
	SHA256    string
	FetchedAt time.Time
	SizeBytes int64
}

type DatasetStats struct {
	Total          int            `json:"total"`
	Labeled        int            `json:"labeled"`
	Unlabeled      int            `json:"unlabeled"`
	ByClass        map[string]int `json:"by_class"`
	TotalSizeBytes int64          `json:"total_size_bytes"`
}

func (s *Store) UpsertImage(id, path, sha256 string, fetchedAt time.Time, sizeBytes int64) error {
	_, err := s.DB.Exec(
		`INSERT INTO images(id, path, sha256, fetched_at, size_bytes)
		 VALUES(?, ?, ?, ?, ?)
		 ON CONFLICT(sha256) DO UPDATE SET path=excluded.path, fetched_at=excluded.fetched_at, size_bytes=excluded.size_bytes`,
		id, path, sha256, fetchedAt.UTC().Format(time.RFC3339), sizeBytes,
	)
	return err
}

func (s *Store) SetLabel(imageID, skystate string, meteor bool, labeledAt time.Time) error {
	m := 0
	if meteor {
		m = 1
	}
	_, err := s.DB.Exec(
		`INSERT INTO labels(image_id, skystate, meteor, labeled_at)
		 VALUES(?, ?, ?, ?)
		 ON CONFLICT(image_id) DO UPDATE SET skystate=excluded.skystate, meteor=excluded.meteor, labeled_at=excluded.labeled_at`,
		imageID, skystate, m, labeledAt.UTC().Format(time.RFC3339),
	)
	return err
}

// ClearLabels deletes all labels; images remain untouched.
func (s *Store) ClearLabels() error {
	_, err := s.DB.Exec(`DELETE FROM labels`)
	if err != nil {
		return fmt.Errorf("clear labels: %w", err)
	}
	return nil
}

func (s *Store) GetLabel(imageID string) (skystate string, meteor bool, ok bool, err error) {
	var m int
	var w string
	row := s.DB.QueryRow(`SELECT skystate, meteor FROM labels WHERE image_id = ?`, imageID)
	switch e := row.Scan(&w, &m); {
	case e == sql.ErrNoRows:
		return "", false, false, nil
	case e != nil:
		return "", false, false, e
	default:
		return w, m == 1, true, nil
	}
}

func (s *Store) CountLabeled() (int, error) {
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM labels`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count labels: %w", err)
	}
	return n, nil
}

// CountStats returns basic dataset counters.
func (s *Store) CountStats() (DatasetStats, error) {
	var stats DatasetStats

	stats.ByClass = map[string]int{
		"clear":         0,
		"light_clouds":  0,
		"heavy_clouds":  0,
		"precipitation": 0,
		"unknown":       0,
	}

	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM images`).Scan(&stats.Total); err != nil {
		return stats, fmt.Errorf("count images: %w", err)
	}
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM labels`).Scan(&stats.Labeled); err != nil {
		return stats, fmt.Errorf("count labels: %w", err)
	}

	rows, err := s.DB.Query(`SELECT skystate, COUNT(*) FROM labels GROUP BY skystate`)
	if err != nil {
		return stats, fmt.Errorf("count by class: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var n int
		if err := rows.Scan(&name, &n); err != nil {
			return stats, fmt.Errorf("scan class count: %w", err)
		}
		stats.ByClass[name] = n
	}
	if err := rows.Err(); err != nil {
		return stats, fmt.Errorf("rows: %w", err)
	}

	if err := s.DB.QueryRow(`
SELECT COUNT(*)
FROM images i
LEFT JOIN labels l ON l.image_id = i.id
WHERE l.image_id IS NULL`).Scan(&stats.Unlabeled); err != nil {
		return stats, fmt.Errorf("count unlabeled: %w", err)
	}

	if err := s.DB.QueryRow(`SELECT COALESCE(SUM(size_bytes), 0) FROM images`).Scan(&stats.TotalSizeBytes); err != nil {
		return stats, fmt.Errorf("sum sizes: %w", err)
	}

	return stats, nil
}

type ImageWithLabel struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	SHA256    string    `json:"sha256"`
	FetchedAt time.Time `json:"fetched_at"`
	SizeBytes int64     `json:"size_bytes"`

	Skystate  *string    `json:"skystate,omitempty"`
	Meteor    *bool      `json:"meteor,omitempty"`
	LabeledAt *time.Time `json:"labeled_at,omitempty"`
}

func (s *Store) ListImages(limit int, unlabeledOnly bool, day string) ([]ImageWithLabel, error) {
	useLimit := limit > 0

	var q string
	var args []any
	var where []string

	if day != "" {
		where = append(where, "DATE(i.fetched_at) = ?")
		args = append(args, day)
	}

	if unlabeledOnly {
		q = `
SELECT i.id, i.path, i.sha256, i.fetched_at, i.size_bytes,
       NULL as skystate, NULL as meteor, NULL as labeled_at
FROM images i
LEFT JOIN labels l ON l.image_id = i.id
`
		where = append(where, "l.image_id IS NULL")
	} else {
		q = `
SELECT i.id, i.path, i.sha256, i.fetched_at, i.size_bytes,
       l.skystate, l.meteor, l.labeled_at
FROM images i
LEFT JOIN labels l ON l.image_id = i.id
`
	}

	if len(where) > 0 {
		q += "WHERE " + strings.Join(where, " AND ") + "\n"
	}

	q += "ORDER BY i.fetched_at DESC"

	if useLimit {
		q += "\nLIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	defer rows.Close()

	var out []ImageWithLabel
	for rows.Next() {
		var (
			id, path, sha256, fetchedAtStr string
			sizeBytes                      int64
			skystateNS                     sql.NullString
			meteorNI                       sql.NullInt64
			labeledAtNS                    sql.NullString
		)

		if err := rows.Scan(&id, &path, &sha256, &fetchedAtStr, &sizeBytes, &skystateNS, &meteorNI, &labeledAtNS); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}

		fetchedAt, err := time.Parse(time.RFC3339, fetchedAtStr)
		if err != nil {
			// If parsing fails, still return something deterministic
			fetchedAt = time.Time{}
		}

		item := ImageWithLabel{
			ID:        id,
			Path:      path,
			SHA256:    sha256,
			FetchedAt: fetchedAt,
			SizeBytes: sizeBytes,
		}

		if skystateNS.Valid {
			s := skystateNS.String
			item.Skystate = &s
		}
		if meteorNI.Valid {
			m := meteorNI.Int64 == 1
			item.Meteor = &m
		}
		if labeledAtNS.Valid {
			tm, err := time.Parse(time.RFC3339, labeledAtNS.String)
			if err == nil {
				item.LabeledAt = &tm
			}
		}

		out = append(out, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	return out, nil
}

type DaySummary struct {
	Date      string `json:"date"`
	Count     int    `json:"count"`
	SizeBytes int64  `json:"size_bytes"`
}

// ListDays returns available days (UTC) with counts and total size, newest first.
func (s *Store) ListDays() ([]DaySummary, error) {
	rows, err := s.DB.Query(`
SELECT DATE(fetched_at) as day, COUNT(*) as cnt, COALESCE(SUM(size_bytes), 0) as total_size
FROM images
GROUP BY day
ORDER BY day DESC`)
	if err != nil {
		return nil, fmt.Errorf("list days: %w", err)
	}
	defer rows.Close()

	var out []DaySummary
	for rows.Next() {
		var d DaySummary
		if err := rows.Scan(&d.Date, &d.Count, &d.SizeBytes); err != nil {
			return nil, fmt.Errorf("scan day: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// CleanupResult holds the result of a cleanup operation
type CleanupResult struct {
	DeletedCount int      `json:"deleted_count"`
	DeletedPaths []string `json:"deleted_paths"`
	FreedBytes   int64    `json:"freed_bytes"`
}

// GetOldestUnlabeledImages returns the oldest unlabeled images (by fetched_at)
func (s *Store) GetOldestUnlabeledImages(limit int) ([]ImageWithLabel, error) {
	q := `
SELECT i.id, i.path, i.sha256, i.fetched_at, i.size_bytes,
       NULL as skystate, NULL as meteor, NULL as labeled_at
FROM images i
LEFT JOIN labels l ON l.image_id = i.id
WHERE l.image_id IS NULL
ORDER BY i.fetched_at ASC
LIMIT ?`

	rows, err := s.DB.Query(q, limit)
	if err != nil {
		return nil, fmt.Errorf("get oldest unlabeled: %w", err)
	}
	defer rows.Close()

	var out []ImageWithLabel
	for rows.Next() {
		var (
			id, path, sha256, fetchedAtStr string
			sizeBytes                      int64
			skystateNS                     sql.NullString
			meteorNI                       sql.NullInt64
			labeledAtNS                    sql.NullString
		)
		if err := rows.Scan(&id, &path, &sha256, &fetchedAtStr, &sizeBytes, &skystateNS, &meteorNI, &labeledAtNS); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		fetchedAt, _ := time.Parse(time.RFC3339, fetchedAtStr)
		out = append(out, ImageWithLabel{
			ID:        id,
			Path:      path,
			SHA256:    sha256,
			FetchedAt: fetchedAt,
			SizeBytes: sizeBytes,
		})
	}
	return out, rows.Err()
}

// DeleteImage removes an image from the database (labels are cascade deleted)
func (s *Store) DeleteImage(id string) error {
	_, err := s.DB.Exec(`DELETE FROM images WHERE id = ?`, id)
	return err
}

// CountUnlabeled returns the number of unlabeled images
func (s *Store) CountUnlabeled() (int, error) {
	var n int
	err := s.DB.QueryRow(`
SELECT COUNT(*)
FROM images i
LEFT JOIN labels l ON l.image_id = i.id
WHERE l.image_id IS NULL`).Scan(&n)
	return n, err
}

// CountUnlabeledByDay returns unlabeled image count for a specific day
func (s *Store) CountUnlabeledByDay(day string) (int, error) {
	var n int
	err := s.DB.QueryRow(`
SELECT COUNT(*)
FROM images i
LEFT JOIN labels l ON l.image_id = i.id
WHERE l.image_id IS NULL AND DATE(i.fetched_at) = ?`, day).Scan(&n)
	return n, err
}

// GetUnlabeledByDay returns all unlabeled images for a specific day
func (s *Store) GetUnlabeledByDay(day string) ([]ImageWithLabel, error) {
	q := `
SELECT i.id, i.path, i.sha256, i.fetched_at, i.size_bytes,
       NULL as skystate, NULL as meteor, NULL as labeled_at
FROM images i
LEFT JOIN labels l ON l.image_id = i.id
WHERE l.image_id IS NULL AND DATE(i.fetched_at) = ?
ORDER BY i.fetched_at ASC`

	rows, err := s.DB.Query(q, day)
	if err != nil {
		return nil, fmt.Errorf("get unlabeled by day: %w", err)
	}
	defer rows.Close()

	var out []ImageWithLabel
	for rows.Next() {
		var (
			id, path, sha256, fetchedAtStr string
			sizeBytes                      int64
			skystateNS                     sql.NullString
			meteorNI                       sql.NullInt64
			labeledAtNS                    sql.NullString
		)
		if err := rows.Scan(&id, &path, &sha256, &fetchedAtStr, &sizeBytes, &skystateNS, &meteorNI, &labeledAtNS); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		fetchedAt, _ := time.Parse(time.RFC3339, fetchedAtStr)
		out = append(out, ImageWithLabel{
			ID:        id,
			Path:      path,
			SHA256:    sha256,
			FetchedAt: fetchedAt,
			SizeBytes: sizeBytes,
		})
	}
	return out, rows.Err()
}

// DeleteUnlabeledByDay deletes all unlabeled images for a specific day and returns cleanup result
func (s *Store) DeleteUnlabeledByDay(day string) (CleanupResult, error) {
	images, err := s.GetUnlabeledByDay(day)
	if err != nil {
		return CleanupResult{}, err
	}

	result := CleanupResult{
		DeletedPaths: make([]string, 0, len(images)),
	}

	for _, img := range images {
		if err := s.DeleteImage(img.ID); err != nil {
			return result, fmt.Errorf("delete image %s: %w", img.ID, err)
		}
		result.DeletedCount++
		result.DeletedPaths = append(result.DeletedPaths, img.Path)
		result.FreedBytes += img.SizeBytes
	}

	return result, nil
}

// DeleteOldestUnlabeled deletes the oldest N unlabeled images to keep count under maxUnlabeled
func (s *Store) DeleteOldestUnlabeled(maxUnlabeled int) (CleanupResult, error) {
	count, err := s.CountUnlabeled()
	if err != nil {
		return CleanupResult{}, err
	}

	if count <= maxUnlabeled {
		return CleanupResult{}, nil
	}

	toDelete := count - maxUnlabeled
	images, err := s.GetOldestUnlabeledImages(toDelete)
	if err != nil {
		return CleanupResult{}, err
	}

	result := CleanupResult{
		DeletedPaths: make([]string, 0, len(images)),
	}

	for _, img := range images {
		if err := s.DeleteImage(img.ID); err != nil {
			return result, fmt.Errorf("delete image %s: %w", img.ID, err)
		}
		result.DeletedCount++
		result.DeletedPaths = append(result.DeletedPaths, img.Path)
		result.FreedBytes += img.SizeBytes
	}

	return result, nil
}
