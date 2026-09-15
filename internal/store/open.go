package store

import (
	"database/sql"
	"fmt"
)

// Open ouvre la base locale et la met au schema attendu.
//
// Les pragmas sont poses a l'ouverture plutot que dans une migration : ils
// decrivent la facon dont ce programme veut utiliser le fichier, pas le
// schema. WAL laisse les lectures se poursuivre pendant qu'un releve s'ecrit,
// ce qui compte sur une carte SD ou l'ecriture est lente. foreign_keys est
// desactive par defaut dans SQLite et doit etre demande a chaque connexion,
// sans quoi les ON DELETE CASCADE du schema ne s'appliqueraient pas.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open(driverName, path)
	if err != nil {
		return nil, fmt.Errorf("ouverture de %s: %w", path, err)
	}
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("%s: %w", p, err)
		}
	}
	if err := Migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
