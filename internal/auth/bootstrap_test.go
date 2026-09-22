package auth

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/floopich/mire-go/internal/store"
)

func baseEtDossier(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "mire.db"))
	if err != nil {
		t.Fatalf("ouverture: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, dir
}

func muet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestUnMotDePasseEstGenereAuPremierDemarrage(t *testing.T) {
	db, dir := baseEtDossier(t)
	ctx := context.Background()

	if err := Bootstrap(ctx, db, dir, muet()); err != nil {
		t.Fatalf("amorcage: %v", err)
	}
	hash, _ := store.Setting(ctx, db, KeyPasswordHash, "")
	if hash == "" {
		t.Fatal("aucune empreinte enregistree")
	}

	raw, err := os.ReadFile(filepath.Join(dir, InitialPasswordFile))
	if err != nil {
		t.Fatalf("fichier absent: %v", err)
	}
	password := string(raw[:len(raw)-1])
	ok, err := Check(ctx, db, password)
	if err != nil || !ok {
		t.Error("le mot de passe du fichier ne correspond pas a l'empreinte")
	}

	info, _ := os.Stat(filepath.Join(dir, InitialPasswordFile))
	if info.Mode().Perm() != 0o600 {
		t.Errorf("permissions %v, attendu 0600", info.Mode().Perm())
	}
}

func TestLAmorcageNeRemplacePasUnMotDePasseExistant(t *testing.T) {
	db, dir := baseEtDossier(t)
	ctx := context.Background()

	if err := Bootstrap(ctx, db, dir, muet()); err != nil {
		t.Fatal(err)
	}
	avant, _ := store.Setting(ctx, db, KeyPasswordHash, "")
	if err := Bootstrap(ctx, db, dir, muet()); err != nil {
		t.Fatal(err)
	}
	apres, _ := store.Setting(ctx, db, KeyPasswordHash, "")
	if avant != apres {
		t.Error("le mot de passe a ete regenere au deuxieme demarrage")
	}
}

func TestDeuxInstancesOntDesMotsDePasseDifferents(t *testing.T) {
	// Un defaut partage serait lisible dans un depot public et rendrait toute
	// instance non configuree attaquable d'avance.
	a, dirA := baseEtDossier(t)
	b, dirB := baseEtDossier(t)
	ctx := context.Background()
	_ = Bootstrap(ctx, a, dirA, muet())
	_ = Bootstrap(ctx, b, dirB, muet())

	ha, _ := store.Setting(ctx, a, KeyPasswordHash, "")
	hb, _ := store.Setting(ctx, b, KeyPasswordHash, "")
	if ha == hb {
		t.Error("deux instances partagent la meme empreinte")
	}
}

func TestChangerLeMotDePasseEffaceLaTraceEtLesSessions(t *testing.T) {
	db, dir := baseEtDossier(t)
	ctx := context.Background()
	_ = Bootstrap(ctx, db, dir, muet())

	sessions := NewSessions(db)
	token, _ := sessions.Create(ctx)

	if err := SetPassword(ctx, db, dir, "un-nouveau-mot-de-passe"); err != nil {
		t.Fatalf("changement: %v", err)
	}
	if InitialPasswordPending(dir) {
		t.Error("le fichier du mot de passe genere a survecu")
	}
	if sessions.Valid(ctx, token) == nil {
		t.Error("une session ouverte avec l'ancien mot de passe survit")
	}
	if ok, _ := Check(ctx, db, "un-nouveau-mot-de-passe"); !ok {
		t.Error("le nouveau mot de passe ne fonctionne pas")
	}
}

func TestMotDePasseTropCourtRefuse(t *testing.T) {
	db, dir := baseEtDossier(t)
	if err := SetPassword(context.Background(), db, dir, "court"); err == nil {
		t.Error("un mot de passe de cinq caracteres devrait etre refuse")
	}
}

func TestSansEmpreinteAucunMotDePasseNePasse(t *testing.T) {
	// Une base sans empreinte ne doit pas laisser entrer, pas meme avec une
	// chaine vide.
	db, _ := baseEtDossier(t)
	ctx := context.Background()
	for _, essai := range []string{"", "admin", "x"} {
		if ok, _ := Check(ctx, db, essai); ok {
			t.Errorf("%q accepte sans empreinte enregistree", essai)
		}
	}
}
