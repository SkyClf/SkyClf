package store

import (
	"database/sql"
	"fmt"
	"time"
)

type LatestRow struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	SHA256    string    `json:"sha256"`
	FetchedAt time.Time `json:"fetched_at"`

	SkyState  *string    `json:"skystate,omitempty"`
	Meteor    *bool      `json:"meteor,omitempty"`
	LabeledAt *time.Time `json:"labeled_at,omitempty"`
}

func (s *Store) GetLatest() (*LatestRow, error) {
	row := s.DB.QueryRow(`
SELECT i.id, i.path, i.sha256, i.fetched_at,
       l.skystate, l.meteor, l.labeled_at
FROM images i
LEFT JOIN labels l ON l.image_id = i.id
ORDER BY i.fetched_at DESC
LIMIT 1;
`)

	var (
		id, path, sha256, fetchedAtStr string
		skyNS                          sql.NullString
		meteorNI                       sql.NullInt64
		labeledAtNS                    sql.NullString
	)

	if err := row.Scan(&id, &path, &sha256, &fetchedAtStr, &skyNS, &meteorNI, &labeledAtNS); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get latest: %w", err)
	}

	fetchedAt, _ := time.Parse(time.RFC3339, fetchedAtStr)

	out := &LatestRow{
		ID:        id,
		Path:      path,
		SHA256:    sha256,
		FetchedAt: fetchedAt,
	}

	if skyNS.Valid {
		v := skyNS.String
		out.SkyState = &v
	}
	if meteorNI.Valid {
		m := meteorNI.Int64 == 1
		out.Meteor = &m
	}
	if labeledAtNS.Valid {
		if tm, err := time.Parse(time.RFC3339, labeledAtNS.String); err == nil {
			out.LabeledAt = &tm
		}
	}

	return out, nil
}
