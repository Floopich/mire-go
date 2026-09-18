package store

import (
	"context"
	"database/sql"
	"strconv"
)

// Setting lit un reglage, ou renvoie la valeur par defaut s'il est absent.
func Setting(ctx context.Context, db *sql.DB, key, fallback string) (string, error) {
	var v string
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return fallback, nil
	}
	if err != nil {
		return fallback, err
	}
	return v, nil
}

// SetSetting enregistre un reglage.
func SetSetting(ctx context.Context, db *sql.DB, key, value string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO settings(key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// BoolSetting lit un reglage booleen.
//
// Une valeur illisible retombe sur la valeur par defaut plutot que de faire
// echouer l'analyse : un reglage corrompu ne doit pas empecher de collecter.
func BoolSetting(ctx context.Context, db *sql.DB, key string, fallback bool) bool {
	raw, err := Setting(ctx, db, key, "")
	if err != nil || raw == "" {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return v
}

// SetBoolSetting enregistre un reglage booleen.
func SetBoolSetting(ctx context.Context, db *sql.DB, key string, value bool) error {
	return SetSetting(ctx, db, key, strconv.FormatBool(value))
}

// KeyOFDMALowQAMExpected declare que le segment fonctionne en basse modulation
// OFDMA par conception.
//
// Un segment configure ainsi depuis l'installation n'est pas un segment qui se
// degrade : sans cette declaration, l'abonne verrait une alerte permanente
// qu'aucune intervention ne ferait disparaitre.
const KeyOFDMALowQAMExpected = "ofdma_low_qam_expected"
