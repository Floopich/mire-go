package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestReglageAbsentRenvoieLeDefaut(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "mire.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	if v, _ := Setting(ctx, db, "inexistant", "repli"); v != "repli" {
		t.Errorf("%q, attendu le repli", v)
	}
	if BoolSetting(ctx, db, KeyOFDMALowQAMExpected, false) {
		t.Error("le reglage doit etre desactive par defaut")
	}
}

func TestReglageEnregistreEtRelu(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "mire.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	if err := SetBoolSetting(ctx, db, KeyOFDMALowQAMExpected, true); err != nil {
		t.Fatalf("ecriture: %v", err)
	}
	if !BoolSetting(ctx, db, KeyOFDMALowQAMExpected, false) {
		t.Error("le reglage n'a pas ete relu")
	}
	// Une deuxieme ecriture remplace la premiere plutot que d'echouer.
	if err := SetBoolSetting(ctx, db, KeyOFDMALowQAMExpected, false); err != nil {
		t.Fatalf("mise a jour: %v", err)
	}
	if BoolSetting(ctx, db, KeyOFDMALowQAMExpected, true) {
		t.Error("la mise a jour n'a pas pris")
	}
}

func TestValeurCorrompueRetombeSurLeDefaut(t *testing.T) {
	// Un reglage illisible ne doit pas empecher de collecter.
	db, err := Open(filepath.Join(t.TempDir(), "mire.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	if err := SetSetting(ctx, db, KeyOFDMALowQAMExpected, "peut-etre"); err != nil {
		t.Fatal(err)
	}
	if !BoolSetting(ctx, db, KeyOFDMALowQAMExpected, true) {
		t.Error("une valeur illisible doit retomber sur le defaut")
	}
}
