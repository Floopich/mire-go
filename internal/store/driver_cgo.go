//go:build !purego

package store

// Pilote SQLite par defaut, adosse a la bibliotheque C.
//
// Il sert au developpement et aux tests. Les binaires distribues sont
// compiles avec -tags purego, qui lui substitue une implementation en Go pur :
// la compilation croisee vers arm64 et armv7 se fait alors sans chaine C, et
// le binaire reste statique.
import _ "github.com/mattn/go-sqlite3"

const driverName = "sqlite3"
