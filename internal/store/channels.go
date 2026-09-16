package store

import (
	"context"
	"database/sql"
	"time"
)

// ChannelSummary resume l'etat d'un canal au dernier releve.
type ChannelSummary struct {
	ChannelID  int
	Modulation string
	Power      *float64
	SNR        *float64
	LastSeen   time.Time
}

// DownstreamChannels liste les canaux descendants vus sur la periode.
//
// On part du dernier releve de chaque canal plutot que d'une liste figee : un
// canal qui disparait apres une reconfiguration du segment doit cesser
// d'apparaitre, et un canal ajoute doit apparaitre sans intervention.
func DownstreamChannels(ctx context.Context, db *sql.DB, since time.Time) ([]ChannelSummary, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT c.channel_id, c.modulation, c.power_dbmv, c.snr_db, c.ts
		FROM ds_channels c
		JOIN (SELECT channel_id, max(ts) AS ts FROM ds_channels
		      WHERE ts >= ? GROUP BY channel_id) d
		  ON c.channel_id = d.channel_id AND c.ts = d.ts
		ORDER BY c.channel_id`, since.UTC().Unix())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []ChannelSummary
	for rows.Next() {
		var s ChannelSummary
		var ts int64
		if err := rows.Scan(&s.ChannelID, &s.Modulation, &s.Power, &s.SNR, &ts); err != nil {
			return nil, err
		}
		s.LastSeen = time.Unix(ts, 0).UTC()
		out = append(out, s)
	}
	return out, rows.Err()
}

// ReadingCount compte les releves enregistres.
func ReadingCount(ctx context.Context, db *sql.DB) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT count(*) FROM readings`).Scan(&n)
	return n, err
}
