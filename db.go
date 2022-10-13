package bow

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	_ "github.com/mattn/go-sqlite3"
)

// DB represents the database connection.
type DB struct {
	db   *sql.DB
	dsn  string // data source name
	fsys fs.FS  // filesystem for migration files
}

// NewDB creates a new DB taking a data source name
// and a filesystem for migration files.
func NewDB(dns string, fsys fs.FS) *DB {
	return &DB{
		dsn:  dns,
		fsys: fsys,
	}
}

// open opens a sqlite database specified by the data source name.
// It also enables WAL mode and foreign keys check, and finally execute
// pending SQL migrations.
func (db *DB) Open() (err error) {
	if db.dsn == "" {
		return fmt.Errorf("dsn required")
	}

	// Create parent directory
	if db.dsn != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(db.dsn), 0700); err != nil {
			return err
		}
	}

	if db.db, err = sql.Open("sqlite3", db.dsn); err != nil {
		return err
	}

	// Enable WAL. Performs better because multiple readers can operate
	// while data is being written.
	if _, err := db.db.Exec(`PRAGMA journal_mode = wal;`); err != nil {
		return fmt.Errorf("enable wal: %w", err)
	}

	// When in WAL mode, setting the synchronous flag to NORMAL is a good
	// choice for most applications. This mode ensures that fsync() calls
	// are only called when the WAL becomes full and has execute a checkpoint
	// to the main database file. This is safe as the WAL is append only.
	if _, err := db.db.Exec(`PRAGMA synchronous = NORMAL;`); err != nil {
		return fmt.Errorf("set synchronous to normal: %w", err)
	}

	// Enable foreign key checks because it is not enabled by default.
	if _, err = db.db.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		return fmt.Errorf("enable foreign keys check: %w", err)
	}

	// Configure the busy timeout to help external processes such as litestream
	// to acquire a write lock in case the application already has a lock.
	if _, err = db.db.Exec(`PRAGMA busy_timeout = 5000;`); err != nil {
		return fmt.Errorf("set busy timeout: %w", err)
	}

	if err := db.migrate(); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	return nil
}

// migrate executes pending migration files.
func (db *DB) migrate() error {
	// Ensure migration table exists.
	if _, err := db.db.Exec(`CREATE TABLE IF NOT EXISTS migrations (name TEXT PRIMARY KEY);`); err != nil {
		return fmt.Errorf("cannot create migrations table: %w", err)
	}

	names, err := fs.Glob(db.fsys, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)

	for _, name := range names {
		if err := db.migrateFile(name); err != nil {
			return fmt.Errorf("migration error: name=%q err=%w", name, err)
		}
	}

	return nil
}

// migrateFile runs a single migration file within a transaction.
func (db *DB) migrateFile(name string) error {
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Ensure migration has not already been run.
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM migrations WHERE name = ?`, name).Scan(&n); err != nil {
		return err
	} else if n != 0 {
		return nil // already run migration, skip
	}

	// Read and execute migration file.
	if buf, err := fs.ReadFile(db.fsys, name); err != nil {
		return err
	} else if _, err := tx.Exec(string(buf)); err != nil {
		return err
	}

	// Insert record into migrations to prevent re-running migration.
	if _, err := tx.Exec(`INSERT INTO migrations (name) VALUES (?)`, name); err != nil {
		return err
	}

	return tx.Commit()
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.db.Close()
}

// BeginTx calls the underlying method on the db.
func (db *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return db.db.BeginTx(ctx, opts)
}
