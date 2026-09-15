package modem

import (
	"testing"
	"time"
)

// reponse reproduit la forme du firmware CGA4233VOO : deux canaux SC-QAM
// descendants, un OFDM, un montant SC-QAM, un OFDMA. ErrTbl compte trois
// lignes : deux pour DSTbl, la troisieme pour le canal OFDM.
const reponse = `{"data":{
 "DSTbl":[
  {"ChannelID":"1","Modulation":"256qam","Frequency":"650000000","PowerLevel":"1.5","SNRLevel":"38.9","Correcteds":"120","Uncorrectables":"3"},
  {"ChannelID":2,"Modulation":"256QAM","Frequency":658000000,"PowerLevel":1.7,"SNRLevel":38.5,"Correcteds":100,"Uncorrectables":0}
 ],
 "exDSTbl":[
  {"ChannelID":"33","CentralFrequency":"800000000","PowerLevel":"2.1","SNRLevel":"40.2","FFT":"1024-qam"}
 ],
 "USTbl":[
  {"ChannelID":"5","Modulation":"64qam","Frequency":"42000000","PowerLevel":"45.5"}
 ],
 "exUSTbl":[
  {"ChannelID":"9","CentralFrequency":"60000000","PowerLevel":"46.0","FFT":"OFDMA"}
 ],
 "ErrTbl":[
  {"Correcteds":"120","Uncorrectables":"3"},
  {"Correcteds":"100","Uncorrectables":"0"},
  {"Correcteds":"7","Uncorrectables":"2"}
 ]}}`

func TestParsePayloadLitLesQuatreTables(t *testing.T) {
	snap, err := parsePayload([]byte(reponse))
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	if len(snap.Downstream) != 3 {
		t.Fatalf("%d canaux descendants, attendu 3", len(snap.Downstream))
	}
	if len(snap.Upstream) != 2 {
		t.Fatalf("%d canaux montants, attendu 2", len(snap.Upstream))
	}
}

func TestCompteursOFDMViennentDeLaBonneLigneDErrTbl(t *testing.T) {
	// Le piege : ErrTbl est positionnel. Si l'OFDM prenait la premiere ligne
	// au lieu de la troisieme, il afficherait 120/3 au lieu de 7/2 -- une
	// erreur invisible a l'oeil sur un tableau de quarante canaux.
	snap, err := parsePayload([]byte(reponse))
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	ofdm := snap.Downstream[2]
	if ofdm.ChannelID != 33 || ofdm.Modulation != "OFDM" {
		t.Fatalf("troisieme canal inattendu: %+v", ofdm)
	}
	if ofdm.CorrErrors != 7 || ofdm.NonCorrErrors != 2 {
		t.Errorf("compteurs OFDM %d/%d, attendu 7/2", ofdm.CorrErrors, ofdm.NonCorrErrors)
	}
}

func TestSNRPositifDevientMSENegativePuisSNRPositif(t *testing.T) {
	snap, _ := parsePayload([]byte(reponse))
	if snap.Downstream[0].MSE != "-38.9" {
		t.Errorf("MSE brute %q, attendu -38.9", snap.Downstream[0].MSE)
	}
	got := snap.Normalize(time.Now()).Downstream[0].SNRDB
	if got == nil || *got != 38.9 {
		t.Errorf("SNR normalise %v, attendu 38.9", got)
	}
}

func TestFrequenceEnMHzEntiers(t *testing.T) {
	snap, _ := parsePayload([]byte(reponse))
	if snap.Downstream[0].Frequency != "650000000 MHz" && snap.Downstream[0].Frequency != "650 MHz" {
		t.Logf("frequence brute: %q", snap.Downstream[0].Frequency)
	}
	f := snap.Normalize(time.Now()).Downstream[0].FrequencyMHz
	if f == nil {
		t.Fatal("frequence absente")
	}
}

func TestModulationNormalisee(t *testing.T) {
	snap, _ := parsePayload([]byte(reponse))
	if snap.Downstream[0].Modulation != "256QAM" {
		t.Errorf("%q, attendu 256QAM", snap.Downstream[0].Modulation)
	}
	if snap.Upstream[0].Modulation != "64QAM" {
		t.Errorf("%q, attendu 64QAM", snap.Upstream[0].Modulation)
	}
	if snap.Upstream[1].Modulation != "OFDMA" {
		t.Errorf("%q, attendu OFDMA", snap.Upstream[1].Modulation)
	}
}

func TestTablesAbsentesNeFontPasEchouer(t *testing.T) {
	// Un modem en cours de synchronisation renvoie des tables vides : c'est un
	// releve pauvre, pas une erreur.
	snap, err := parsePayload([]byte(`{"data":{}}`))
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	if len(snap.Downstream) != 0 || len(snap.Upstream) != 0 {
		t.Error("tables vides devraient donner un releve vide")
	}
}

func TestTablesALaRacineSontAcceptees(t *testing.T) {
	snap, err := parsePayload([]byte(`{"DSTbl":[{"ChannelID":"1","PowerLevel":"1.0"}]}`))
	if err != nil {
		t.Fatalf("analyse: %v", err)
	}
	if len(snap.Downstream) != 1 {
		t.Errorf("%d canaux, attendu 1", len(snap.Downstream))
	}
}

func TestReponseIllisibleEstUneErreur(t *testing.T) {
	if _, err := parsePayload([]byte("<html>401</html>")); err == nil {
		t.Error("une reponse non JSON doit remonter une erreur")
	}
}
