// Package analyze qualifie l'etat d'une ligne a partir des seuils du profil.
//
// L'analyse ne juge jamais une valeur absente : un canal sans mesure est
// inconnu, pas defaillant. C'est la distinction qui evite qu'un modem en cours
// de synchronisation soit rapporte comme une panne.
package analyze

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/floopich/mire-go/internal/docsis"
	"github.com/floopich/mire-go/internal/operator"
)

// Level qualifie un etat, du meilleur au pire.
type Level int

const (
	Unknown Level = iota
	Good
	Tolerated
	Warning
	Critical
)

func (l Level) String() string {
	switch l {
	case Good:
		return "bon"
	case Tolerated:
		return "tolere"
	case Warning:
		return "avertissement"
	case Critical:
		return "critique"
	default:
		return "inconnu"
	}
}

// worst retient le plus severe des deux, Unknown ne masquant jamais un verdict.
func worst(a, b Level) Level {
	if a > b {
		return a
	}
	return b
}

// Finding explique un verdict en une phrase.
type Finding struct {
	Level   Level
	Metric  string
	Message string
}

// ChannelHealth est le verdict porte sur un canal.
type ChannelHealth struct {
	ChannelID int
	Level     Level
	Findings  []Finding
}

// Health est le verdict porte sur un releve complet.
type Health struct {
	Level      Level
	Downstream []ChannelHealth
	Upstream   []ChannelHealth
	Qualified  bool // false si le profil n'est pas calibre
}

// Snapshot analyse un releve.
//
// Un profil non calibre ne produit aucun verdict : appliquer les tolerances
// d'un autre reseau donnerait un resultat faux avec l'apparence du serieux.
func Snapshot(snap docsis.Snapshot, profile operator.Profile) Health {
	h := Health{Qualified: profile.Calibrated}
	if !profile.Calibrated {
		return h
	}
	t := profile.Thresholds
	for _, c := range snap.Downstream {
		ch := downstream(c, t)
		h.Downstream = append(h.Downstream, ch)
		h.Level = worst(h.Level, ch.Level)
	}
	for _, c := range snap.Upstream {
		ch := upstream(c, t)
		h.Upstream = append(h.Upstream, ch)
		h.Level = worst(h.Level, ch.Level)
	}
	return h
}

func downstream(c docsis.Downstream, t operator.Thresholds) ChannelHealth {
	out := ChannelHealth{ChannelID: c.ChannelID}

	if bands, ok := t.DownstreamPowerFor(c.Modulation); ok && c.PowerDBmV != nil {
		f := power(*c.PowerDBmV, bands, "puissance descendante")
		out.Findings = append(out.Findings, f)
		out.Level = worst(out.Level, f.Level)
	}
	if bands, ok := t.SNRFor(c.Modulation); ok && c.SNRDB != nil {
		f := snr(*c.SNRDB, bands)
		out.Findings = append(out.Findings, f)
		out.Level = worst(out.Level, f.Level)
	}
	if f, ok := errors(c, t.Errors); ok {
		out.Findings = append(out.Findings, f)
		out.Level = worst(out.Level, f.Level)
	}
	return out
}

func upstream(c docsis.Upstream, t operator.Thresholds) ChannelHealth {
	out := ChannelHealth{ChannelID: c.ChannelID}

	if bands, ok := t.UpstreamPowerFor(kind(c.Modulation)); ok && c.PowerDBmV != nil {
		f := power(*c.PowerDBmV, bands, "puissance montante")
		out.Findings = append(out.Findings, f)
		out.Level = worst(out.Level, f.Level)
	}
	limits := t.UpstreamModulation
	if kind(c.Modulation) == "ofdma" {
		limits = t.UpstreamOFDMA
	}
	if f, ok := modulation(c.Modulation, limits); ok {
		out.Findings = append(out.Findings, f)
		out.Level = worst(out.Level, f.Level)
	}
	return out
}

// power qualifie une puissance par intervalles emboites.
func power(v float64, b operator.PowerBands, label string) Finding {
	switch {
	case b.Good.Contains(v):
		return Finding{Good, label, fmt.Sprintf("%.1f dBmV, dans la plage nominale", v)}
	case b.Warning.Contains(v):
		return Finding{Warning, label, fmt.Sprintf("%.1f dBmV, hors plage nominale", v)}
	case b.Critical.Contains(v):
		return Finding{Critical, label, fmt.Sprintf("%.1f dBmV, proche des limites", v)}
	default:
		return Finding{Critical, label, fmt.Sprintf("%.1f dBmV, hors limites", v)}
	}
}

func snr(v float64, b operator.SNRBands) Finding {
	switch {
	case v >= b.GoodMin:
		return Finding{Good, "SNR", fmt.Sprintf("%.1f dB", v)}
	case v >= b.WarningMin:
		return Finding{Warning, "SNR", fmt.Sprintf("%.1f dB, sous le seuil nominal", v)}
	case v >= b.CriticalMin:
		return Finding{Critical, "SNR", fmt.Sprintf("%.1f dB, degrade", v)}
	default:
		return Finding{Critical, "SNR", fmt.Sprintf("%.1f dB, tres degrade", v)}
	}
}

// errors qualifie la proportion d'erreurs non corrigees.
//
// On raisonne en proportion et non en valeur absolue : les compteurs sont
// cumulatifs depuis le dernier redemarrage du modem, donc un grand nombre
// n'indique rien par lui-meme.
func errors(c docsis.Downstream, limits operator.ErrorLimits) (Finding, bool) {
	total := c.CorrErrors + c.NonCorrErrors
	if total == 0 {
		return Finding{}, false
	}
	pct := float64(c.NonCorrErrors) / float64(total) * 100
	switch {
	case pct >= limits.UncorrectablePctCritical:
		return Finding{Critical, "erreurs",
			fmt.Sprintf("%.1f %% non corrigees", pct)}, true
	case pct >= limits.UncorrectablePctWarning:
		return Finding{Warning, "erreurs",
			fmt.Sprintf("%.1f %% non corrigees", pct)}, true
	default:
		return Finding{Good, "erreurs", fmt.Sprintf("%.1f %% non corrigees", pct)}, true
	}
}

// modulation qualifie la modulation montante.
//
// Un canal durablement bas peut relever de la configuration du segment plutot
// que d'une degradation : le verdict signale, il ne conclut pas.
func modulation(m string, limits operator.ModulationLimits) (Finding, bool) {
	qam := qamOrder(m)
	if qam == 0 {
		return Finding{}, false
	}
	switch {
	case qam <= limits.CriticalMaxQAM:
		return Finding{Critical, "modulation montante",
			fmt.Sprintf("%s, tres basse", m)}, true
	case qam <= limits.WarningMaxQAM:
		return Finding{Warning, "modulation montante",
			fmt.Sprintf("%s, basse : comparer l'historique avant de conclure", m)}, true
	case limits.ToleratedMaxQAM > 0 && qam <= limits.ToleratedMaxQAM:
		return Finding{Tolerated, "modulation montante", m}, true
	default:
		return Finding{Good, "modulation montante", m}, true
	}
}

// qamOrder extrait l'ordre de modulation : "64QAM" donne 64.
func qamOrder(m string) int {
	s := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(m), " ", ""))
	s = strings.TrimSuffix(s, "QAM")
	s = strings.TrimPrefix(s, "QAM")
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// kind distingue les familles de canaux montants.
func kind(m string) string {
	if strings.Contains(strings.ToLower(m), "ofdma") {
		return "ofdma"
	}
	return "sc_qam"
}
