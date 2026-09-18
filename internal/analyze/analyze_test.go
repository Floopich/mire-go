package analyze

import (
	"testing"
	"time"

	"github.com/floopich/mire-go/internal/docsis"
	"github.com/floopich/mire-go/internal/operator"
)

func profilVOO(t *testing.T) operator.Profile {
	t.Helper()
	p, err := operator.Load("voo")
	if err != nil {
		t.Fatalf("profil: %v", err)
	}
	return p
}

func releve(ds []docsis.RawDownstream, us []docsis.RawUpstream) docsis.Snapshot {
	return docsis.RawSnapshot{DOCSIS: "3.1", Downstream: ds, Upstream: us}.
		Normalize(time.Now())
}

func TestLigneSaineEstBonne(t *testing.T) {
	// Les valeurs relevees sur une ligne VOO en bon etat.
	h := Snapshot(releve(
		[]docsis.RawDownstream{{ChannelID: 1, Modulation: "256QAM",
			PowerLevel: "3.8", MSE: "-41.2"}},
		[]docsis.RawUpstream{{ChannelID: 5, Modulation: "64QAM", PowerLevel: "45.5"}},
	), profilVOO(t))

	if h.Level != Good {
		t.Errorf("verdict %s, attendu bon : %+v", h.Level, h.Downstream[0].Findings)
	}
	if !h.Qualified {
		t.Error("le profil VOO est calibre, l'analyse doit etre qualifiee")
	}
}

func TestPuissanceMontanteAuPlafondEstCritique(t *testing.T) {
	// Chez VOO, les canaux tombent au-dela de 52 dBmV. C'est le defaut le
	// plus frequent sur une voie retour degradee.
	h := Snapshot(releve(nil,
		[]docsis.RawUpstream{{ChannelID: 5, Modulation: "64QAM", PowerLevel: "53.0"}},
	), profilVOO(t))

	if h.Level != Critical {
		t.Errorf("verdict %s, attendu critique", h.Level)
	}
}

func TestPuissanceMontanteNormaleSous49(t *testing.T) {
	h := Snapshot(releve(nil,
		[]docsis.RawUpstream{{ChannelID: 5, Modulation: "64QAM", PowerLevel: "48.0"}},
	), profilVOO(t))
	if h.Level != Good {
		t.Errorf("48 dBmV devrait etre bon, obtenu %s", h.Level)
	}
}

func TestCanalOFDMJugeSurSesPropresSeuils(t *testing.T) {
	// Sans ligne ofdm dediee, 27 dB seraient juges sur le plancher de 40 dB
	// du 4096QAM et ressortiraient en critique alors que le canal va bien.
	h := Snapshot(releve(
		[]docsis.RawDownstream{{ChannelID: 159, Modulation: "OFDM",
			PowerLevel: "7.5", MSE: "-27.5"}}, nil), profilVOO(t))

	if h.Level != Good {
		t.Errorf("verdict %s, attendu bon : %+v", h.Level, h.Downstream[0].Findings)
	}
}

func TestSNRDegradeEnQAMEleve(t *testing.T) {
	h := Snapshot(releve(
		[]docsis.RawDownstream{{ChannelID: 1, Modulation: "4096QAM",
			PowerLevel: "3.0", MSE: "-34.0"}}, nil), profilVOO(t))
	if h.Level != Critical {
		t.Errorf("34 dB en 4096QAM devrait etre critique, obtenu %s", h.Level)
	}
}

func TestValeurAbsenteResteInconnueEtNePenalisePas(t *testing.T) {
	// Un modem en cours de synchronisation ne doit pas etre rapporte en panne.
	h := Snapshot(releve(
		[]docsis.RawDownstream{{ChannelID: 1, Modulation: "256QAM",
			PowerLevel: "", MSE: ""}}, nil), profilVOO(t))

	if h.Level != Unknown {
		t.Errorf("verdict %s, attendu inconnu", h.Level)
	}
	if len(h.Downstream[0].Findings) != 0 {
		t.Errorf("aucun constat attendu, obtenu %+v", h.Downstream[0].Findings)
	}
}

func TestModulationMontanteBasseAvertitSansConclure(t *testing.T) {
	h := Snapshot(releve(nil,
		[]docsis.RawUpstream{{ChannelID: 5, Modulation: "16QAM", PowerLevel: "45.0"}},
	), profilVOO(t))

	if h.Level != Warning {
		t.Errorf("16QAM devrait avertir, obtenu %s", h.Level)
	}
	var trouve bool
	for _, f := range h.Upstream[0].Findings {
		if f.Metric == "modulation montante" {
			trouve = true
			// Le message doit inviter a comparer l'historique : un segment
			// configure en 16QAM depuis l'installation n'est pas degrade.
			if f.Message == "16QAM" {
				t.Error("le message devrait nuancer le verdict")
			}
		}
	}
	if !trouve {
		t.Error("aucun constat sur la modulation")
	}
}

func TestErreursEnProportionPasEnValeurAbsolue(t *testing.T) {
	// Les compteurs sont cumulatifs depuis le dernier redemarrage : un million
	// d'erreurs corrigees sur dix millions n'indique rien de mauvais.
	sain := Snapshot(releve([]docsis.RawDownstream{{ChannelID: 1,
		Modulation: "256QAM", PowerLevel: "3.0", MSE: "-40.0",
		CorrErrors: 1_000_000, NonCorrErrors: 100}}, nil), profilVOO(t))
	if sain.Level != Good {
		t.Errorf("0,01 %% non corrigees devrait etre bon, obtenu %s", sain.Level)
	}

	mauvais := Snapshot(releve([]docsis.RawDownstream{{ChannelID: 1,
		Modulation: "256QAM", PowerLevel: "3.0", MSE: "-40.0",
		CorrErrors: 900, NonCorrErrors: 100}}, nil), profilVOO(t))
	if mauvais.Level != Critical {
		t.Errorf("10 %% non corrigees devrait etre critique, obtenu %s", mauvais.Level)
	}
}

func TestProfilNonCalibreNeQualifieRien(t *testing.T) {
	p, err := operator.Load("generique")
	if err != nil {
		t.Fatal(err)
	}
	h := Snapshot(releve(
		[]docsis.RawDownstream{{ChannelID: 1, Modulation: "256QAM",
			PowerLevel: "99.0", MSE: "-5.0"}}, nil), p)

	if h.Qualified {
		t.Error("un profil non calibre ne doit pas se declarer qualifie")
	}
	if h.Level != Unknown || len(h.Downstream) != 0 {
		t.Errorf("aucun verdict attendu, obtenu %s avec %d canaux", h.Level, len(h.Downstream))
	}
}

func TestLePireCanalDetermineLeVerdictGlobal(t *testing.T) {
	h := Snapshot(releve([]docsis.RawDownstream{
		{ChannelID: 1, Modulation: "256QAM", PowerLevel: "3.0", MSE: "-40.0"},
		{ChannelID: 2, Modulation: "256QAM", PowerLevel: "20.0", MSE: "-40.0"},
	}, nil), profilVOO(t))

	if h.Level != Critical {
		t.Errorf("verdict global %s, un canal hors limites devrait le rendre critique", h.Level)
	}
	if h.Downstream[0].Level != Good {
		t.Error("le canal sain ne doit pas etre degrade par son voisin")
	}
}
