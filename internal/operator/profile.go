// Package operator porte les profils d'operateur.
//
// Tout ce qui varie d'un operateur a l'autre est une donnee, pas du code :
// l'adresse par defaut du modem, les modeles attendus, et les seuils qui
// qualifient l'etat de la ligne. Ajouter un operateur revient a ecrire un
// fichier JSON dans profiles/, pas a toucher au programme.
package operator

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

//go:embed profiles/*.json
var profileFS embed.FS

// Range est un intervalle ferme, en dBmV.
type Range struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// Contains indique si une valeur tombe dans l'intervalle.
func (r Range) Contains(v float64) bool { return v >= r.Min && v <= r.Max }

// PowerBands qualifie une puissance par intervalles emboites.
type PowerBands struct {
	Good     Range `json:"good"`
	Warning  Range `json:"warning"`
	Critical Range `json:"critical"`
}

// SNRBands qualifie un rapport signal/bruit par planchers.
//
// Contrairement a la puissance, seul le bas compte : un SNR trop eleve n'est
// pas un defaut.
type SNRBands struct {
	GoodMin     float64 `json:"good_min"`
	WarningMin  float64 `json:"warning_min"`
	CriticalMin float64 `json:"critical_min"`
}

// ModulationLimits borne la modulation acceptable en voie montante.
type ModulationLimits struct {
	CriticalMaxQAM  int `json:"critical_max_qam"`
	WarningMaxQAM   int `json:"warning_max_qam"`
	ToleratedMaxQAM int `json:"tolerated_max_qam,omitempty"`
}

// ErrorLimits borne la proportion d'erreurs non corrigees.
type ErrorLimits struct {
	UncorrectablePctWarning  float64 `json:"uncorrectable_pct_warning"`
	UncorrectablePctCritical float64 `json:"uncorrectable_pct_critical"`
	SpikeExpiryHours         int     `json:"spike_expiry_hours"`
}

// Thresholds est le jeu de seuils complet d'un profil.
//
// Les cartes sont indexees par modulation ("256QAM", "ofdm", "sc_qam"...).
// DefaultKey donne la modulation a utiliser quand le modem en annonce une
// qu'on ne connait pas : mieux vaut un seuil approchant qu'aucune analyse.
type Thresholds struct {
	DownstreamPower    map[string]PowerBands `json:"downstream_power"`
	DownstreamDefault  string                `json:"downstream_default"`
	UpstreamPower      map[string]PowerBands `json:"upstream_power"`
	UpstreamDefault    string                `json:"upstream_default"`
	SNR                map[string]SNRBands   `json:"snr"`
	SNRDefault         string                `json:"snr_default"`
	UpstreamModulation ModulationLimits      `json:"upstream_modulation"`
	UpstreamOFDMA      ModulationLimits      `json:"upstream_ofdma"`
	Errors             ErrorLimits           `json:"errors"`
}

// Profile decrit un operateur et la facon d'analyser ses lignes.
type Profile struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Region          string     `json:"region"`
	DefaultModemURL string     `json:"default_modem_url"`
	ExpectedModels  []string   `json:"expected_models"`
	Calibrated      bool       `json:"calibrated"`
	Source          string     `json:"source,omitempty"`
	Notes           string     `json:"notes,omitempty"`
	Thresholds      Thresholds `json:"thresholds"`
}

// DownstreamPowerFor renvoie les bandes de puissance descendante applicables.
func (t Thresholds) DownstreamPowerFor(modulation string) (PowerBands, bool) {
	return lookup(t.DownstreamPower, modulation, t.DownstreamDefault)
}

// UpstreamPowerFor renvoie les bandes de puissance montante applicables.
func (t Thresholds) UpstreamPowerFor(modulation string) (PowerBands, bool) {
	return lookup(t.UpstreamPower, modulation, t.UpstreamDefault)
}

// SNRFor renvoie les planchers de SNR applicables.
func (t Thresholds) SNRFor(modulation string) (SNRBands, bool) {
	return lookup(t.SNR, modulation, t.SNRDefault)
}

func lookup[T any](table map[string]T, key, fallback string) (T, bool) {
	var zero T
	if len(table) == 0 {
		return zero, false
	}
	if v, ok := table[normalize(key)]; ok {
		return v, true
	}
	if v, ok := table[fallback]; ok {
		return v, true
	}
	return zero, false
}

// normalize ramene les variantes d'ecriture du modem a une cle unique.
//
// Le CGA4233 ecrit "256QAM", d'autres firmwares "256 QAM" ou "qam256". On
// tolere les trois plutot que de perdre l'analyse sur un espace.
func normalize(modulation string) string {
	s := strings.ToLower(strings.TrimSpace(modulation))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	if strings.HasPrefix(s, "qam") && len(s) > 3 {
		s = s[3:] + "qam"
	}
	if strings.HasSuffix(s, "qam") {
		return strings.ToUpper(strings.TrimSuffix(s, "qam")) + "QAM"
	}
	return s
}

// Load renvoie le profil demande.
func Load(id string) (Profile, error) {
	data, err := profileFS.ReadFile("profiles/" + id + ".json")
	if err != nil {
		return Profile{}, fmt.Errorf("profil %q inconnu", id)
	}
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return Profile{}, fmt.Errorf("profil %q illisible: %w", id, err)
	}
	if p.ID != id {
		return Profile{}, fmt.Errorf("profil %q declare l'identifiant %q", id, p.ID)
	}
	return p, nil
}

// All renvoie tous les profils embarques, tries par identifiant.
func All() ([]Profile, error) {
	entries, err := profileFS.ReadDir("profiles")
	if err != nil {
		return nil, err
	}
	var out []Profile
	for _, e := range entries {
		id := strings.TrimSuffix(e.Name(), ".json")
		p, err := Load(id)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
