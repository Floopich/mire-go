// Commande mire-view : sert une vue en lecture seule des releves.
//
// Cette vue existe pour verifier que la collecte est fidele : elle affiche ce
// qui est en base, sans rien interpreter. Elle n'ecrit jamais.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/floopich/mire-go/internal/operator"
	"github.com/floopich/mire-go/internal/store"
	"github.com/floopich/mire-go/internal/webview"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		os.Exit(1)
	}
}

func run() error {
	dbPath := flag.String("db", "mire.db", "chemin de la base a lire")
	addr := flag.String("ecoute", "127.0.0.1:1341", "adresse d'ecoute")
	profileID := flag.String("profil", "voo", "profil d'operateur")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	profile, err := operator.Load(*profileID)
	if err != nil {
		return err
	}
	db, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	srv, err := webview.NewServer(db, profile)
	if err != nil {
		return err
	}

	// Ecoute sur la boucle locale par defaut : cette vue n'a aucune
	// authentification, l'exposer au reseau publierait l'etat de la ligne a
	// qui passe par la.
	server := &http.Server{
		Addr:              *addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()

	logger.Info("vue disponible", "adresse", "http://"+*addr, "base", *dbPath)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	logger.Info("vue arretee")
	return nil
}
