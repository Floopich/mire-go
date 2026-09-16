// Package webview rend les releves sous forme de pages lisibles.
//
// Les courbes sont du SVG assemble cote serveur : aucun JavaScript, aucun
// fichier externe, tout part dans la reponse. C'est volontairement minimal --
// cette vue existe pour verifier que la collecte est fidele, pas pour etre
// l'interface definitive.
package webview

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/floopich/mire-go/internal/store"
)

const (
	chartWidth  = 720
	chartHeight = 220
	padLeft     = 46
	padRight    = 12
	padTop      = 12
	padBottom   = 26
)

// Series est une courbe a tracer.
type Series struct {
	Label  string
	Points []store.Point
	// Value extrait la valeur a tracer d'un point : puissance ou SNR.
	Value func(store.Point) *float64
	Unit  string
}

// SVG assemble un graphique.
//
// Les points sans valeur interrompent le trace au lieu d'etre relies : une
// ligne droite au-dessus d'un trou de mesure ferait croire a une continuite
// qui n'a pas ete observee.
func SVG(s Series) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %d %d" role="img" aria-label="%s" class="chart">`,
		chartWidth, chartHeight, escape(s.Label))

	known := knownValues(s)
	if len(known) < 2 {
		fmt.Fprintf(&b, `<text x="%d" y="%d" class="empty">Pas assez de points</text></svg>`,
			chartWidth/2-70, chartHeight/2)
		return b.String()
	}

	lo, hi := bounds(known)
	t0 := s.Points[0].Timestamp.Unix()
	t1 := s.Points[len(s.Points)-1].Timestamp.Unix()
	if t1 == t0 {
		t1 = t0 + 1
	}

	// Axes et graduations : trois reperes suffisent a lire un ordre de grandeur.
	for i := range 3 {
		v := lo + (hi-lo)*float64(i)/2
		y := yFor(v, lo, hi)
		fmt.Fprintf(&b, `<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" class="grid"/>`,
			padLeft, y, chartWidth-padRight, y)
		fmt.Fprintf(&b, `<text x="4" y="%.1f" class="tick">%.1f</text>`, y+4, v)
	}

	var path strings.Builder
	pen := "M"
	for _, p := range s.Points {
		v := s.Value(p)
		if v == nil {
			pen = "M" // trou de mesure : on leve le crayon
			continue
		}
		x := padLeft + float64(chartWidth-padLeft-padRight)*
			float64(p.Timestamp.Unix()-t0)/float64(t1-t0)
		fmt.Fprintf(&path, "%s%.1f %.1f ", pen, x, yFor(*v, lo, hi))
		pen = "L"
	}
	fmt.Fprintf(&b, `<path d="%s" class="line"/>`, strings.TrimSpace(path.String()))

	fmt.Fprintf(&b, `<text x="%d" y="%d" class="axis">%s</text>`,
		padLeft, chartHeight-6, escape(s.Points[0].Timestamp.Format("02/01 15:04")))
	fmt.Fprintf(&b, `<text x="%d" y="%d" class="axis" text-anchor="end">%s</text>`,
		chartWidth-padRight, chartHeight-6,
		escape(s.Points[len(s.Points)-1].Timestamp.Format("02/01 15:04")))
	b.WriteString(`</svg>`)
	return b.String()
}

func knownValues(s Series) []float64 {
	var out []float64
	for _, p := range s.Points {
		if v := s.Value(p); v != nil {
			out = append(out, *v)
		}
	}
	return out
}

// bounds choisit l'echelle verticale.
//
// Une marge de 10 % evite que la courbe colle aux bords, et un intervalle
// plancher empeche qu'une ligne parfaitement stable soit rendue comme une
// oscillation spectaculaire entre deux valeurs identiques.
func bounds(values []float64) (lo, hi float64) {
	lo, hi = values[0], values[0]
	for _, v := range values {
		lo = math.Min(lo, v)
		hi = math.Max(hi, v)
	}
	if hi-lo < 1 {
		mid := (lo + hi) / 2
		lo, hi = mid-0.5, mid+0.5
	}
	margin := (hi - lo) * 0.1
	return lo - margin, hi + margin
}

func yFor(v, lo, hi float64) float64 {
	usable := float64(chartHeight - padTop - padBottom)
	return padTop + usable*(1-(v-lo)/(hi-lo))
}

// Fmt rend une valeur optionnelle, en distinguant l'absence du zero.
func Fmt(v *float64, unit string) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprintf("%.1f %s", *v, unit)
}

// FmtTime rend un horodatage en heure locale.
func FmtTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("02/01/2006 15:04")
}

func escape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
