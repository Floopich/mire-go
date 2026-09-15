package modem

import (
	"context"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fauxModem rejoue le protocole du firmware CGA4233VOO.
type fauxModem struct {
	motDePasse string
	// expireApres fait repondre 401 aux lectures apres N requetes reussies,
	// pour verifier la reconnexion.
	expireApres  int
	lectures     int
	logins       int
	dernierJeton string
	sansToken    bool // le firmware reel ne renvoie pas de champ "token"
	entetesVus   http.Header
}

func (f *fauxModem) handler() http.Handler {
	const salt, saltWeb = "sel-modem", "sel-webui"
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/session/login", func(w http.ResponseWriter, r *http.Request) {
		f.entetesVus = r.Header.Clone()
		_ = r.ParseForm()
		switch r.Form.Get("password") {
		case "seeksalthash":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"salt":"` + salt + `","saltwebui":"` + saltWeb + `"}`))
		case derive(f.motDePasse, salt, saltWeb):
			f.logins++
			f.dernierJeton = "jeton-" + string(rune('A'+f.logins))
			http.SetCookie(w, &http.Cookie{Name: "auth", Value: f.dernierJeton, Path: "/"})
			if f.sansToken {
				_, _ = w.Write([]byte(`{"error":"ok"}`))
			} else {
				_, _ = w.Write([]byte(`{"error":"ok","token":"` + f.dernierJeton + `"}`))
			}
		default:
			_, _ = w.Write([]byte(`{"error":"MSG_LOGIN_1"}`))
		}
	})

	mux.HandleFunc("/api/v1/session/menu", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})

	mux.HandleFunc("/api/v1/modem/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-CSRF-TOKEN") != f.dernierJeton {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.lectures++
		if f.expireApres > 0 && f.lectures > f.expireApres {
			// Une seule expiration : sinon la reconnexion retomberait
			// aussitot sur un 401 et le test ne prouverait rien.
			f.expireApres = 0
			f.dernierJeton = ""
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(reponse))
	})
	return mux
}

func derive(mdp, salt, saltWeb string) string {
	first, _ := pbkdf2.Key(sha256.New, mdp, []byte(salt), 1000, 16)
	second, _ := pbkdf2.Key(sha256.New, hex.EncodeToString(first), []byte(saltWeb), 1000, 16)
	return hex.EncodeToString(second)
}

func piloteVers(t *testing.T, f *fauxModem) *CGA4233 {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	m, err := NewCGA4233(srv.URL, "voo", f.motDePasse)
	if err != nil {
		t.Fatalf("creation: %v", err)
	}
	return m
}

func TestSequenceDAuthentificationComplete(t *testing.T) {
	f := &fauxModem{motDePasse: "secret"}
	m := piloteVers(t, f)

	snap, err := m.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("releve: %v", err)
	}
	if len(snap.Downstream) != 3 {
		t.Errorf("%d canaux descendants, attendu 3", len(snap.Downstream))
	}
	if f.logins != 1 {
		t.Errorf("%d authentifications, attendu 1", f.logins)
	}
}

func TestLeJetonEstReprisDuCookieQuandLaReponseNEnContientPas(t *testing.T) {
	// C'est le cas du firmware reel : pas de champ "token", seulement le
	// cookie auth. Sans ce repli, modem/* repond 401 indefiniment.
	f := &fauxModem{motDePasse: "secret", sansToken: true}
	m := piloteVers(t, f)

	if _, err := m.Snapshot(context.Background()); err != nil {
		t.Fatalf("releve: %v", err)
	}
	if m.token == "" {
		t.Error("aucun jeton recupere depuis le cookie auth")
	}
}

func TestLaSessionNEstPasRejoueeAChaqueReleve(t *testing.T) {
	f := &fauxModem{motDePasse: "secret"}
	m := piloteVers(t, f)

	for i := range 3 {
		if _, err := m.Snapshot(context.Background()); err != nil {
			t.Fatalf("releve %d: %v", i+1, err)
		}
	}
	if f.logins != 1 {
		t.Errorf("%d authentifications pour 3 releves, attendu 1", f.logins)
	}
}

func TestReconnexionApresExpirationDeSession(t *testing.T) {
	f := &fauxModem{motDePasse: "secret", expireApres: 1}
	m := piloteVers(t, f)

	if _, err := m.Snapshot(context.Background()); err != nil {
		t.Fatalf("premier releve: %v", err)
	}
	if _, err := m.Snapshot(context.Background()); err != nil {
		t.Fatalf("releve apres expiration: %v", err)
	}
	if f.logins != 2 {
		t.Errorf("%d authentifications, attendu 2", f.logins)
	}
}

func TestMauvaisMotDePasseEstUneErreurDAuth(t *testing.T) {
	f := &fauxModem{motDePasse: "secret"}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	m, _ := NewCGA4233(srv.URL, "voo", "mauvais")
	_, err := m.Snapshot(context.Background())
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("attendu ErrAuth, obtenu %v", err)
	}
	// Le message ne doit pas exposer le mot de passe essaye.
	if strings.Contains(err.Error(), "mauvais") {
		t.Errorf("le message fuit le mot de passe: %v", err)
	}
}

func TestLesEntetesExigesParLeFirmwareSontEnvoyes(t *testing.T) {
	f := &fauxModem{motDePasse: "secret"}
	m := piloteVers(t, f)
	if _, err := m.Snapshot(context.Background()); err != nil {
		t.Fatalf("releve: %v", err)
	}
	for _, h := range []string{"User-Agent", "X-Requested-With", "Referer"} {
		if f.entetesVus.Get(h) == "" {
			t.Errorf("en-tete %s absent", h)
		}
	}
	if f.entetesVus.Get("X-Requested-With") != "XMLHttpRequest" {
		t.Errorf("X-Requested-With = %q", f.entetesVus.Get("X-Requested-With"))
	}
}

func TestAdresseVideEstRefusee(t *testing.T) {
	if _, err := NewCGA4233("  ", "voo", "x"); err == nil {
		t.Error("une adresse vide doit etre refusee a la creation")
	}
}

func TestContexteAnnuleInterromptLeReleve(t *testing.T) {
	f := &fauxModem{motDePasse: "secret"}
	m := piloteVers(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Snapshot(ctx); err == nil {
		t.Error("un contexte annule doit interrompre le releve")
	}
}
