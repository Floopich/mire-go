package webview

import (
	"database/sql"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/floopich/mire-go/internal/analyze"
	"github.com/floopich/mire-go/internal/operator"
	"github.com/floopich/mire-go/internal/store"
)

// Server sert une vue en lecture seule de la base.
type Server struct {
	db      *sql.DB
	profile operator.Profile
	tmpl    *template.Template
}

func NewServer(db *sql.DB, profile operator.Profile) (*Server, error) {
	tmpl, err := template.New("page").Funcs(template.FuncMap{
		"fmtValue": Fmt,
		"fmtTime":  FmtTime,
	}).Parse(pageHTML)
	if err != nil {
		return nil, err
	}
	return &Server{db: db, profile: profile, tmpl: tmpl}, nil
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /canal/{id}", s.handleChannel)
	return mux
}

type pageData struct {
	Profile  operator.Profile
	Readings int
	Latest   time.Time
	Health   analyze.Health
	Channels []store.ChannelSummary
	Upstream []store.ChannelSummary
	Channel  *store.ChannelSummary
	PowerSVG template.HTML
	SNRSVG   template.HTML
	Hours    int
	Err      string
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := pageData{Profile: s.profile, Hours: hoursParam(r)}

	count, err := store.ReadingCount(ctx, s.db)
	if err != nil {
		s.fail(w, r, data, err)
		return
	}
	data.Readings = count

	since := time.Now().Add(-time.Duration(data.Hours) * time.Hour)
	channels, err := store.DownstreamChannels(ctx, s.db, since)
	if err != nil {
		s.fail(w, r, data, err)
		return
	}
	data.Channels = channels
	if len(channels) > 0 {
		data.Latest = channels[0].LastSeen
	}

	montants, err := store.UpstreamChannels(ctx, s.db, since)
	if err != nil {
		s.fail(w, r, data, err)
		return
	}
	data.Upstream = montants

	// Le verdict porte sur le dernier releve complet : qualifier des valeurs
	// venant de releves differents melangerait des instants distincts.
	if snap, err := store.LatestSnapshot(ctx, s.db); err == nil {
		data.Health = analyze.SnapshotWith(snap, s.profile, analyze.Options{
			OFDMALowQAMExpected: store.BoolSetting(ctx, s.db, store.KeyOFDMALowQAMExpected, false),
		})
	}
	s.render(w, r, data)
}

func (s *Server) handleChannel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := pageData{Profile: s.profile, Hours: hoursParam(r)}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "numero de canal invalide", http.StatusBadRequest)
		return
	}
	to := time.Now()
	from := to.Add(-time.Duration(data.Hours) * time.Hour)

	points, err := store.DownstreamHistory(ctx, s.db, id, from, to)
	if err != nil {
		s.fail(w, r, data, err)
		return
	}
	data.Channel = &store.ChannelSummary{ChannelID: id}
	if len(points) > 0 {
		last := points[len(points)-1]
		data.Channel.Power = last.Power
		data.Channel.SNR = last.SNR
		data.Channel.LastSeen = last.Timestamp
	}
	data.PowerSVG = template.HTML(SVG(Series{
		Label: "Puissance du canal " + strconv.Itoa(id), Points: points, Unit: "dBmV",
		Value: func(p store.Point) *float64 { return p.Power },
	}))
	data.SNRSVG = template.HTML(SVG(Series{
		Label: "SNR du canal " + strconv.Itoa(id), Points: points, Unit: "dB",
		Value: func(p store.Point) *float64 { return p.SNR },
	}))
	s.render(w, r, data)
}

// hoursParam borne la fenetre demandee.
//
// Sans borne, un parametre absurde ferait balayer dix ans de releves pour
// dessiner une courbe de sept cents pixels de large.
func hoursParam(r *http.Request) int {
	h, err := strconv.Atoi(r.URL.Query().Get("heures"))
	if err != nil || h <= 0 {
		return 24
	}
	if h > 24*90 {
		return 24 * 90
	}
	return h
}

func (s *Server) render(w http.ResponseWriter, _ *http.Request, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.Execute(w, data); err != nil {
		http.Error(w, "rendu impossible", http.StatusInternalServerError)
	}
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, data pageData, err error) {
	data.Err = err.Error()
	w.WriteHeader(http.StatusInternalServerError)
	s.render(w, r, data)
}
