package auth

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/floopich/mire-go/internal/store"
)

// KeyPasswordHash est la cle du reglage portant l'empreinte administrateur.
const KeyPasswordHash = "admin_password_hash"

// InitialPasswordFile porte le mot de passe genere, le temps que l'abonne le
// relise.
const InitialPasswordFile = ".initial_password"

// Bootstrap garantit qu'un mot de passe existe au demarrage.
//
// Sans mot de passe, il n'y a aucune authentification : n'importe qui sur le
// reseau pourrait changer l'adresse et les identifiants du modem. Un mot de
// passe par defaut partage serait pire encore, puisqu'il serait lisible dans
// un depot public. On en genere donc un par instance.
//
// Il n'est revele que par des canaux qui exigent deja un acces a la machine :
// le journal et un fichier en 0600. Jamais dans une page web, la page de
// connexion etant par construction accessible sans authentification.
func Bootstrap(ctx context.Context, db *sql.DB, dataDir string, logger *slog.Logger) error {
	existing, err := store.Setting(ctx, db, KeyPasswordHash, "")
	if err != nil {
		return err
	}
	if existing != "" {
		return nil
	}

	password, err := GeneratePassword()
	if err != nil {
		return err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	if err := store.SetSetting(ctx, db, KeyPasswordHash, hash); err != nil {
		return err
	}

	stored := writeSecret(filepath.Join(dataDir, InitialPasswordFile), password)
	logger.Warn("aucun mot de passe administrateur : Mire en a genere un pour cette instance",
		"mot_de_passe", password)
	if stored {
		logger.Warn("il est aussi lisible dans le fichier du dossier de donnees",
			"fichier", InitialPasswordFile)
	} else {
		logger.Warn("notez-le maintenant : il n'est ecrit nulle part ailleurs")
	}
	return nil
}

// SetPassword remplace le mot de passe et ferme toutes les sessions.
func SetPassword(ctx context.Context, db *sql.DB, dataDir, password string) error {
	if len(strings.TrimSpace(password)) < 8 {
		return fmt.Errorf("mot de passe trop court : huit caracteres au minimum")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	if err := store.SetSetting(ctx, db, KeyPasswordHash, hash); err != nil {
		return err
	}
	// La trace du mot de passe genere ne doit pas survivre a son remplacement.
	_ = os.Remove(filepath.Join(dataDir, InitialPasswordFile))
	return NewSessions(db).RevokeAll(ctx)
}

// Check verifie un mot de passe contre l'empreinte enregistree.
func Check(ctx context.Context, db *sql.DB, password string) (bool, error) {
	hash, err := store.Setting(ctx, db, KeyPasswordHash, "")
	if err != nil || hash == "" {
		return false, err
	}
	return VerifyPassword(password, hash)
}

// InitialPasswordPending dit si le mot de passe genere n'a pas encore ete
// remplace, pour le rappeler sur la page de connexion.
func InitialPasswordPending(dataDir string) bool {
	_, err := os.Stat(filepath.Join(dataDir, InitialPasswordFile))
	return err == nil
}

// writeSecret ecrit le mot de passe en 0600, de facon atomique.
func writeSecret(path, value string) bool {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return false
	}
	_, werr := f.WriteString(value + "\n")
	cerr := f.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmp)
		return false
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return false
	}
	return os.Chmod(path, 0o600) == nil
}
