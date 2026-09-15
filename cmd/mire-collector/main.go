// Commande mire-collector : interroge un modem cable et conserve ses releves.
//
// Elle ne sert pas les donnees : c'est volontaire. Tant que la collecte n'a
// pas fait ses preuves en parallele d'une installation existante, une
// interface ne prouverait rien.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/floopich/mire-go/internal/collect"
	"github.com/floopich/mire-go/internal/modem"
	"github.com/floopich/mire-go/internal/operator"
	"github.com/floopich/mire-go/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		dbPath    = flag.String("db", "mire.db", "chemin de la base locale")
		profileID = flag.String("profil", "voo", "profil d'operateur")
		modemURL  = flag.String("modem", "", "adresse du modem (defaut : celle du profil)")
		user      = flag.String("utilisateur", "voo", "identifiant du modem")
		interval  = flag.Duration("intervalle", 10*time.Minute, "delai entre deux releves")
		retention = flag.Int("retention", store.DefaultRetention.Days, "conservation en jours, 0 pour illimite")
		maxGB     = flag.Float64("taille-max", 5, "plafond de la base en Go, 0 pour illimite")
		once      = flag.Bool("une-fois", false, "prendre un seul releve et sortir")
		verbose   = flag.Bool("verbeux", false, "journaliser chaque releve")
	)
	flag.Parse()

	// Le mot de passe ne passe pas par la ligne de commande : elle est lisible
	// par tout utilisateur de la machine via /proc.
	password := os.Getenv("MIRE_MODEM_PASSWORD")
	if password == "" {
		return errors.New("definir MIRE_MODEM_PASSWORD avec le mot de passe du modem")
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	profile, err := operator.Load(*profileID)
	if err != nil {
		return err
	}
	address := strings.TrimSpace(*modemURL)
	if address == "" {
		address = profile.DefaultModemURL
	}
	if !profile.Calibrated {
		logger.Warn("profil non calibre : les releves sont conserves mais l'etat de la ligne ne sera pas qualifie",
			"profil", profile.ID)
	}

	driver, err := modem.NewCGA4233(address, *user, password)
	if err != nil {
		return err
	}

	db, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	opts := collect.Options{
		Interval: *interval,
		Retention: store.Retention{
			Days:     *retention,
			MaxBytes: int64(*maxGB * float64(1<<30)),
		},
		Logger: logger,
	}
	collector := collect.New(driver, db, opts)

	if *once {
		if err := collector.CollectOnce(context.Background()); err != nil {
			return err
		}
		logger.Info("releve enregistre", "base", *dbPath)
		return nil
	}

	// SIGINT et SIGTERM arretent la boucle proprement : un releve en cours
	// se termine ou expire, et la base se ferme sans laisser de journal WAL
	// a rejouer au demarrage suivant.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("collecte demarree",
		"modem", address, "profil", profile.ID,
		"intervalle", opts.Interval.String(), "base", *dbPath)

	if err := collector.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	logger.Info("collecte arretee")
	return nil
}
