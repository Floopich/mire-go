//go:build purego

package store

// Pilote SQLite en Go pur, utilise pour les binaires distribues.
//
// Meme interface database/sql que la variante cgo, seul le nom enregistre
// change. Compiler avec -tags purego.
import _ "modernc.org/sqlite"

const driverName = "sqlite"
