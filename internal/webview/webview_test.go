package webview

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/floopich/mire-go/internal/docsis"
	"github.com/floopich/mire-go/internal/operator"
	"github.com/floopich/mire-go/internal/store"
)

func baseRemplie(t *testing.T, n int) *sql.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "mire.db"))
	if err != nil {
		t.Fatalf("ouverture: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	debut := time.Now().Add(-time.Duration(n) * time.Minute)
	for i := range n {
		raw := docsis.RawSnapshot{
			DOCSIS: "3.1",
			Downstream: []docsis.RawDownstream{
				{ChannelID: 1, Modulation: "256QAM", Frequency: "650 MHz",
					PowerLevel: "1.5", MSE: "-38.9"},
				{ChannelID: 33, Modulation: "OFDM", Frequency: "800 MHz",
					PowerLevel: "2.1", MSE: ""},
			},
			Upstream: []docsis.RawUpstream{{ChannelID: 5, PowerLevel: "45.5"}},
		}
		at := debut.Add(time.Duration(i) * time.Minute)
		if _, err := store.SaveSnapshot(context.Background(), db, raw.Normalize(at)); err != nil {
			t.Fatalf("releve %d: %v", i, err)
		}
	}
	return db
}

func serveur(t *testing.T, db *sql.DB) http.Handler {
	t.Helper()
	p, err := operator.Load("voo")
	if err != nil {
		t.Fatalf("profil: %v", err)
	}
	s, err := NewServer(db, p)
	if err != nil {
		t.Fatalf("serveur: %v", err)
	}
	return s.Routes()
}

func get(t *testing.T, h http.Handler, url string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	return rec
}

func TestPageIndexListeLesCanaux(t *testing.T) {
	rec := get(t, serveur(t, baseRemplie(t, 10)), "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d", rec.Code)
	}
	body := rec.Body.String()
	for _, attendu := range []string{"Canal", "256QAM", "OFDM", "/canal/1", "/canal/33"} {
		if !strings.Contains(body, attendu) {
			t.Errorf("la page ne contient pas %q", attendu)
		}
	}
}

func TestPageCanalTraceDeuxCourbes(t *testing.T) {
	rec := get(t, serveur(t, baseRemplie(t, 30)), "/canal/1")
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Count(body, "<svg") != 2 {
		t.Errorf("%d graphiques, attendu 2", strings.Count(body, "<svg"))
	}
	if !strings.Contains(body, `class="line"`) {
		t.Error("aucune courbe tracee")
	}
}

func TestAucunJavaScriptNiRessourceExterne(t *testing.T) {
	// La page doit rester lisible sur un reseau sans acces exterieur, et ne
	// rien charger que le binaire n'embarque.
	body := get(t, serveur(t, baseRemplie(t, 10)), "/canal/1").Body.String()
	for _, interdit := range []string{"<script", "http://", "https://", "src="} {
		if strings.Contains(body, interdit) {
			t.Errorf("la page contient %q", interdit)
		}
	}
}

func TestValeurAbsenteAfficheeCommeAbsente(t *testing.T) {
	// Le canal OFDM n'a pas de SNR : il doit apparaitre comme inconnu, pas
	// comme un canal a 0 dB, qui se lirait comme une panne.
	body := get(t, serveur(t, baseRemplie(t, 5)), "/").Body.String()
	if !strings.Contains(body, "—") {
		t.Error("aucune valeur absente signalee")
	}
	if strings.Contains(body, "0.0 dB<") {
		t.Error("une valeur absente est rendue comme 0.0 dB")
	}
}

func TestBaseVideNeCassePas(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "vide.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	rec := get(t, serveur(t, db), "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d sur base vide", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Aucun relevé") {
		t.Error("la page devrait dire qu'il n'y a pas de releve")
	}
}

func TestFenetreAberranteEstBornee(t *testing.T) {
	// Sans borne, dix ans de releves seraient balayes pour dessiner sept
	// cents pixels.
	for _, url := range []string{"/?heures=-5", "/?heures=999999", "/?heures=abc"} {
		if rec := get(t, serveur(t, baseRemplie(t, 3)), url); rec.Code != http.StatusOK {
			t.Errorf("%s: HTTP %d", url, rec.Code)
		}
	}
}

func TestCanalInvalideEstRefuse(t *testing.T) {
	if rec := get(t, serveur(t, baseRemplie(t, 3)), "/canal/abc"); rec.Code != http.StatusBadRequest {
		t.Errorf("HTTP %d, attendu 400", rec.Code)
	}
}

func TestTrouDeMesureInterrompLeTrace(t *testing.T) {
	// Relier deux points de part et d'autre d'un trou ferait croire a une
	// continuite qui n'a pas ete observee.
	maintenant := time.Now()
	points := []store.Point{
		{Timestamp: maintenant, Power: ptr(1)},
		{Timestamp: maintenant.Add(time.Minute), Power: nil},
		{Timestamp: maintenant.Add(2 * time.Minute), Power: ptr(3)},
	}
	svg := SVG(Series{Label: "test", Points: points, Unit: "dBmV",
		Value: func(p store.Point) *float64 { return p.Power }})
	if strings.Count(svg, "M") < 2 {
		t.Errorf("le trace devrait etre interrompu, obtenu: %s", svg)
	}
}

func TestLigneStableNEstPasRenduePlate(t *testing.T) {
	maintenant := time.Now()
	var points []store.Point
	for i := range 5 {
		points = append(points, store.Point{
			Timestamp: maintenant.Add(time.Duration(i) * time.Minute), Power: ptr(1.5)})
	}
	svg := SVG(Series{Label: "stable", Points: points, Unit: "dBmV",
		Value: func(p store.Point) *float64 { return p.Power }})
	if strings.Contains(svg, "NaN") || strings.Contains(svg, "Inf") {
		t.Errorf("echelle degeneree sur une ligne stable: %s", svg)
	}
}

func ptr(v float64) *float64 { return &v }
