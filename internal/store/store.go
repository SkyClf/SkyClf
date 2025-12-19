package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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
	return err
}

type Image struct {
	ID        string
	Path      string
	SHA256    string
	FetchedAt time.Time
}

type DatasetStats struct {
	Total     int            `json:"total"`
	Labeled   int            `json:"labeled"`
	Unlabeled int            `json:"unlabeled"`
	ByClass   map[string]int `json:"by_class"`
}

func (s *Store) UpsertImage(id, path, sha256 string, fetchedAt time.Time) error {
	_, err := s.DB.Exec(
		`INSERT INTO images(id, path, sha256, fetched_at)
		 VALUES(?, ?, ?, ?)
		 ON CONFLICT(sha256) DO UPDATE SET path=excluded.path, fetched_at=excluded.fetched_at`,
		id, path, sha256, fetchedAt.UTC().Format(time.RFC3339),
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

	return stats, nil
}

type ImageWithLabel struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	SHA256    string    `json:"sha256"`
	FetchedAt time.Time `json:"fetched_at"`

	Skystate  *string    `json:"skystate,omitempty"`
	Meteor    *bool      `json:"meteor,omitempty"`
	LabeledAt *time.Time `json:"labeled_at,omitempty"`
}

func (s *Store) ListImages(limit int, unlabeledOnly bool) ([]ImageWithLabel, error) {
	useLimit := limit > 0

	var q string
	var args []any
	if unlabeledOnly {
		q = `
SELECT i.id, i.path, i.sha256, i.fetched_at,
       NULL as skystate, NULL as meteor, NULL as labeled_at
FROM images i
LEFT JOIN labels l ON l.image_id = i.id
WHERE l.image_id IS NULL
ORDER BY i.fetched_at DESC`
	} else {
		q = `
SELECT i.id, i.path, i.sha256, i.fetched_at,
       l.skystate, l.meteor, l.labeled_at
FROM images i
LEFT JOIN labels l ON l.image_id = i.id
ORDER BY i.fetched_at DESC`
	}

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
			skystateNS                     sql.NullString
			meteorNI                       sql.NullInt64
			labeledAtNS                    sql.NullString
		)

		if err := rows.Scan(&id, &path, &sha256, &fetchedAtStr, &skystateNS, &meteorNI, &labeledAtNS); err != nil {
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
