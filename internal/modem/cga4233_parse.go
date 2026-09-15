package modem

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/floopich/mire-go/internal/docsis"
)

// payload est la reponse de /api/v1/modem/... du CGA4233.
//
// Le firmware repond tantot {"data": {...}}, tantot les tables a la racine :
// les deux formes sont acceptees.
type payload struct {
	Data *tables `json:"data"`
	tables
}

type tables struct {
	DSTbl   []dsSCQAM `json:"DSTbl"`
	ExDSTbl []dsOFDM  `json:"exDSTbl"`
	USTbl   []usSCQAM `json:"USTbl"`
	ExUSTbl []usOFDMA `json:"exUSTbl"`
	ErrTbl  []errRow  `json:"ErrTbl"`
}

type dsSCQAM struct {
	ChannelID  docsis.FlexFloat `json:"ChannelID"`
	Modulation string           `json:"Modulation"`
	Frequency  docsis.FlexFloat `json:"Frequency"`
	PowerLevel docsis.FlexFloat `json:"PowerLevel"`
	SNRLevel   docsis.FlexFloat `json:"SNRLevel"`
	errRow
}

type dsOFDM struct {
	ChannelID        docsis.FlexFloat `json:"ChannelID"`
	CentralFrequency docsis.FlexFloat `json:"CentralFrequency"`
	PowerLevel       docsis.FlexFloat `json:"PowerLevel"`
	SNRLevel         docsis.FlexFloat `json:"SNRLevel"`
	FFT              string           `json:"FFT"`
}

type usSCQAM struct {
	ChannelID  docsis.FlexFloat `json:"ChannelID"`
	Modulation string           `json:"Modulation"`
	Frequency  docsis.FlexFloat `json:"Frequency"`
	PowerLevel docsis.FlexFloat `json:"PowerLevel"`
}

type usOFDMA struct {
	ChannelID        docsis.FlexFloat `json:"ChannelID"`
	CentralFrequency docsis.FlexFloat `json:"CentralFrequency"`
	PowerLevel       docsis.FlexFloat `json:"PowerLevel"`
	FFT              string           `json:"FFT"`
}

type errRow struct {
	Correcteds     docsis.FlexFloat `json:"Correcteds"`
	Uncorrectables docsis.FlexFloat `json:"Uncorrectables"`
}

// parsePayload convertit une reponse du modem en releve brut.
func parsePayload(raw []byte) (docsis.RawSnapshot, error) {
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return docsis.RawSnapshot{}, fmt.Errorf("reponse illisible: %w", err)
	}
	t := p.tables
	if p.Data != nil {
		t = *p.Data
	}

	snap := docsis.RawSnapshot{DOCSIS: "3.1"}

	for _, c := range t.DSTbl {
		snap.Downstream = append(snap.Downstream, docsis.RawDownstream{
			ChannelID:     docsis.FlexInt(c.ChannelID),
			Frequency:     mhz(c.Frequency),
			Modulation:    normalizeModulation(c.Modulation),
			PowerLevel:    number(c.PowerLevel),
			MSE:           mse(c.SNRLevel),
			CorrErrors:    docsis.FlexInt(c.Correcteds),
			NonCorrErrors: docsis.FlexInt(c.Uncorrectables),
		})
	}

	// ErrTbl est positionnel : les premieres lignes doublent DSTbl, les
	// suivantes portent les compteurs des canaux OFDM, qui n'ont pas de
	// champ d'erreur propre. Decaler cet index fausserait silencieusement
	// les compteurs de tous les canaux OFDM.
	ofdmErrors := []errRow{}
	if len(t.ErrTbl) > len(t.DSTbl) {
		ofdmErrors = t.ErrTbl[len(t.DSTbl):]
	}
	for i, c := range t.ExDSTbl {
		var e errRow
		if i < len(ofdmErrors) {
			e = ofdmErrors[i]
		}
		snap.Downstream = append(snap.Downstream, docsis.RawDownstream{
			ChannelID:     docsis.FlexInt(c.ChannelID),
			Frequency:     mhz(c.CentralFrequency),
			Modulation:    "OFDM",
			PowerLevel:    number(c.PowerLevel),
			MSE:           mse(c.SNRLevel),
			CorrErrors:    docsis.FlexInt(e.Correcteds),
			NonCorrErrors: docsis.FlexInt(e.Uncorrectables),
		})
	}

	for _, c := range t.USTbl {
		snap.Upstream = append(snap.Upstream, docsis.RawUpstream{
			ChannelID:  docsis.FlexInt(c.ChannelID),
			Frequency:  mhz(c.Frequency),
			Modulation: normalizeModulation(c.Modulation),
			PowerLevel: number(c.PowerLevel),
		})
	}
	for _, c := range t.ExUSTbl {
		snap.Upstream = append(snap.Upstream, docsis.RawUpstream{
			ChannelID:  docsis.FlexInt(c.ChannelID),
			Frequency:  mhz(c.CentralFrequency),
			Modulation: "OFDMA",
			Multiplex:  strings.TrimSpace(c.FFT),
			PowerLevel: number(c.PowerLevel),
		})
	}
	return snap, nil
}

// mhz applique la convention du firmware : frequence en MHz entiers.
func mhz(v docsis.FlexFloat) string {
	if float64(v) == 0 {
		return ""
	}
	return fmt.Sprintf("%d MHz", int(math.Round(float64(v))))
}

func number(v docsis.FlexFloat) string {
	return fmt.Sprintf("%g", float64(v))
}

// mse reproduit la convention du firmware : MSE negative, SNR positif.
//
// Le modem publie un SNR positif ; le modele attend une MSE, et la
// normalisation en reprend la valeur absolue. On rend donc la valeur negative
// ici plutot que de creer une deuxieme convention.
func mse(snr docsis.FlexFloat) string {
	v := math.Abs(float64(snr))
	if v == 0 {
		return ""
	}
	return fmt.Sprintf("%g", -v)
}

// normalizeModulation ramene "256qam", "256 QAM" ou "qam256" a "256QAM".
func normalizeModulation(s string) string {
	t := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	t = strings.ReplaceAll(t, "-", "")
	if strings.HasPrefix(t, "QAM") && len(t) > 3 {
		t = t[3:] + "QAM"
	}
	return t
}
