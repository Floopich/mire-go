package auth

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/floopich/mire-go/internal/store"
)

func TestEmpreinteVerifieLeBonMotDePasse(t *testing.T) {
	h, err := HashPassword("correct-cheval-pile")
	if err != nil {
		t.Fatalf("hachage: %v", err)
	}
	ok, err := VerifyPassword("correct-cheval-pile", h)
	if err != nil || !ok {
		t.Errorf("le bon mot de passe est refuse: %v %v", ok, err)
	}
	ok, _ = VerifyPassword("mauvais", h)
	if ok {
		t.Error("un mauvais mot de passe est accepte")
	}
}

func TestEmpreinteNeContientPasLeMotDePasse(t *testing.T) {
	h, _ := HashPassword("secret-tres-reconnaissable")
	if strings.Contains(h, "secret-tres-reconnaissable") {
		t.Error("le mot de passe apparait en clair dans l'empreinte")
	}
	if !strings.HasPrefix(h, "pbkdf2-sha256$") {
		t.Errorf("format inattendu: %s", h)
	}
}

func TestDeuxEmpreintesDuMemeMotDePasseDifferent(t *testing.T) {
	// Sans sel aleatoire, deux comptes avec le meme mot de passe auraient la
	// meme empreinte, ce qui se voit dans une base volee.
	a, _ := HashPassword("identique")
	b, _ := HashPassword("identique")
	if a == b {
		t.Error("le sel n'est pas aleatoire")
	}
}

func TestEmpreinteCorrompueEstRefusee(t *testing.T) {
	for _, mauvaise := range []string{"", "n'importe quoi", "pbkdf2-sha256$abc$x$y",
		"autre-schema$600000$aaaa$bbbb"} {
		if _, err := VerifyPassword("x", mauvaise); err == nil {
			t.Errorf("%q devrait etre refusee", mauvaise)
		}
	}
}

func TestMotDePasseGenereEstLisibleEtUnique(t *testing.T) {
	vus := map[string]bool{}
	for range 50 {
		p, err := GeneratePassword()
		if err != nil {
			t.Fatal(err)
		}
		if len(p) != 16 {
			t.Fatalf("longueur %d", len(p))
		}
		// Les caracteres confondus a la lecture sont exclus : ce mot de passe
		// sera recopie depuis un journal.
		if strings.ContainsAny(p, "0O1lI") {
			t.Errorf("%q contient un caractere ambigu", p)
		}
		if vus[p] {
			t.Fatal("deux mots de passe identiques generes")
		}
		vus[p] = true
	}
}

func sessionsTest(t *testing.T) *Sessions {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "mire.db"))
	if err != nil {
		t.Fatalf("ouverture: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewSessions(db)
}

func TestSessionCreeePuisValide(t *testing.T) {
	s := sessionsTest(t)
	ctx := context.Background()
	token, err := s.Create(ctx)
	if err != nil {
		t.Fatalf("creation: %v", err)
	}
	if err := s.Valid(ctx, token); err != nil {
		t.Errorf("session refusee: %v", err)
	}
	if err := s.Valid(ctx, "jeton-invente"); err == nil {
		t.Error("un jeton invente est accepte")
	}
	if err := s.Valid(ctx, ""); err == nil {
		t.Error("un jeton vide est accepte")
	}
}

func TestLeJetonNEstPasStockeEnClair(t *testing.T) {
	// Une copie de la base ne doit pas suffire a se connecter a la place de
	// quelqu'un.
	db, err := store.Open(filepath.Join(t.TempDir(), "mire.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	s := NewSessions(db)
	token, _ := s.Create(context.Background())

	var stocke string
	if err := db.QueryRow(`SELECT token_hash FROM sessions`).Scan(&stocke); err != nil {
		t.Fatal(err)
	}
	if stocke == token {
		t.Error("le jeton est stocke tel quel")
	}
}

func TestSessionExpireeEstRefuseePuisPurgee(t *testing.T) {
	s := sessionsTest(t)
	ctx := context.Background()
	token, _ := s.Create(ctx)

	s.now = func() time.Time { return time.Now().Add(SessionLifetime + time.Hour) }
	if err := s.Valid(ctx, token); err == nil {
		t.Error("une session expiree est acceptee")
	}
	if err := s.Prune(ctx); err != nil {
		t.Fatalf("purge: %v", err)
	}
}

func TestRevocationGlobale(t *testing.T) {
	// Apres un changement de mot de passe, une session ouverte par quelqu'un
	// qui connaissait l'ancien ne doit pas survivre trente jours.
	s := sessionsTest(t)
	ctx := context.Background()
	a, _ := s.Create(ctx)
	b, _ := s.Create(ctx)
	if err := s.RevokeAll(ctx); err != nil {
		t.Fatal(err)
	}
	if s.Valid(ctx, a) == nil || s.Valid(ctx, b) == nil {
		t.Error("une session survit a la revocation globale")
	}
}

func TestLimiteurRalentitApresTroisEchecs(t *testing.T) {
	l := NewLimiter()
	if l.Wait("1.2.3.4") != 0 {
		t.Error("une adresse inconnue ne doit pas attendre")
	}
	// Les deux premieres erreurs sont des fautes de frappe.
	l.Fail("1.2.3.4")
	l.Fail("1.2.3.4")
	if l.Wait("1.2.3.4") != 0 {
		t.Error("deux echecs ne devraient pas bloquer")
	}
	l.Fail("1.2.3.4")
	if l.Wait("1.2.3.4") <= 0 {
		t.Error("le troisieme echec devrait imposer un delai")
	}
	// Le delai double.
	premier := l.Wait("1.2.3.4")
	l.Fail("1.2.3.4")
	if l.Wait("1.2.3.4") <= premier {
		t.Error("le delai ne croit pas")
	}
	l.Succeed("1.2.3.4")
	if l.Wait("1.2.3.4") != 0 {
		t.Error("une connexion reussie doit effacer l'historique")
	}
}

func TestLimiteurIsoleLesAdresses(t *testing.T) {
	l := NewLimiter()
	for range 5 {
		l.Fail("10.0.0.1")
	}
	if l.Wait("10.0.0.2") != 0 {
		t.Error("une adresse innocente est bloquee par une autre")
	}
}
