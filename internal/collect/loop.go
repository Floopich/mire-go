// Package collect porte la boucle de releve.
package collect

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/floopich/mire-go/internal/docsis"
	"github.com/floopich/mire-go/internal/store"
)

// Source fournit un releve brut. Le pilote du modem l'implemente.
type Source interface {
	Snapshot(ctx context.Context) (docsis.RawSnapshot, error)
}

// Options regle la boucle.
type Options struct {
	Interval  time.Duration
	Retention store.Retention
	// PurgeEvery espace les purges : les appliquer a chaque releve couterait
	// une lecture de taille de base toutes les dix minutes pour rien.
	PurgeEvery time.Duration
	Logger     *slog.Logger
	// Now est injectable pour les tests.
	Now func() time.Time
}

func (o *Options) normalize() {
	if o.Interval <= 0 {
		o.Interval = 10 * time.Minute
	}
	if o.PurgeEvery <= 0 {
		o.PurgeEvery = 24 * time.Hour
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Retention == (store.Retention{}) {
		o.Retention = store.DefaultRetention
	}
}

// Collector interroge le modem et ecrit ce qu'il lit.
type Collector struct {
	source Source
	db     *sql.DB
	opts   Options
}

func New(source Source, db *sql.DB, opts Options) *Collector {
	opts.normalize()
	return &Collector{source: source, db: db, opts: opts}
}

// Run boucle jusqu'a l'annulation du contexte.
//
// Un premier releve est pris immediatement : attendre dix minutes avant le
// moindre signe de vie donnerait l'impression d'un demarrage rate.
func (c *Collector) Run(ctx context.Context) error {
	ticker := time.NewTicker(c.opts.Interval)
	defer ticker.Stop()

	purgeTicker := time.NewTicker(c.opts.PurgeEvery)
	defer purgeTicker.Stop()

	c.collectOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			c.collectOnce(ctx)
		case <-purgeTicker.C:
			c.purge(ctx)
		}
	}
}

// collectOnce prend un releve et l'enregistre.
//
// Un echec est journalise sans interrompre la boucle : un modem qui redemarre
// ou une coupure ne doivent pas arreter la collecte, ce sont precisement les
// moments qu'on veut documenter.
func (c *Collector) collectOnce(ctx context.Context) {
	// Le releve ne doit pas deborder sur le suivant.
	pollCtx, cancel := context.WithTimeout(ctx, c.opts.Interval)
	defer cancel()

	raw, err := c.source.Snapshot(pollCtx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		c.opts.Logger.Warn("releve echoue", "erreur", err)
		return
	}
	snap := raw.Normalize(c.opts.Now().UTC())
	if _, err := store.SaveSnapshot(ctx, c.db, snap); err != nil {
		c.opts.Logger.Error("enregistrement echoue", "erreur", err)
		return
	}
	c.opts.Logger.Debug("releve enregistre",
		"descendants", len(snap.Downstream), "montants", len(snap.Upstream))
}

func (c *Collector) purge(ctx context.Context) {
	res, err := store.Purge(ctx, c.db, c.opts.Retention, c.opts.Now())
	if err != nil {
		c.opts.Logger.Error("purge echouee", "erreur", err)
		return
	}
	if res.ByAge > 0 || res.BySize > 0 {
		c.opts.Logger.Info("purge", "par_age", res.ByAge, "par_taille", res.BySize)
	}
}

// CollectOnce prend un unique releve. Utile au diagnostic et aux tests.
func (c *Collector) CollectOnce(ctx context.Context) error {
	raw, err := c.source.Snapshot(ctx)
	if err != nil {
		return err
	}
	_, err = store.SaveSnapshot(ctx, c.db, raw.Normalize(c.opts.Now().UTC()))
	return err
}
