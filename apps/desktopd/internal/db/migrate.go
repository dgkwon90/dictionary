package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type migration struct {
	version  int
	contents []byte
	checksum string
}

func Migrate(ctx context.Context, database *sql.DB, log *slog.Logger) error {
	if _, err := database.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(
version INTEGER PRIMARY KEY,
checksum TEXT NOT NULL,
applied_at DATETIME NOT NULL
)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	applied := 0
	for _, item := range migrations {
		wasApplied, err := applyMigration(ctx, database, item)
		if err != nil {
			return err
		}
		if wasApplied {
			applied++
			log.Info("applied database migration", "version", fmt.Sprintf("%04d", item.version))
		}
	}
	if applied == 0 {
		log.Debug("database schema is up to date")
	}

	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}

	items := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		version, err := strconv.Atoi(prefix)
		if err != nil {
			return nil, fmt.Errorf("parse migration version %q: %w", prefix, err)
		}
		contents, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		sum := sha256.Sum256(contents)
		items = append(items, migration{
			version:  version,
			contents: contents,
			checksum: hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].version < items[j].version })
	for index := 1; index < len(items); index++ {
		if items[index-1].version == items[index].version {
			return nil, fmt.Errorf("duplicate migration version %04d", items[index].version)
		}
	}
	return items, nil
}

func applyMigration(ctx context.Context, database *sql.DB, item migration) (applied bool, resultErr error) {
	conn, err := database.Conn(ctx)
	if err != nil {
		return false, fmt.Errorf("acquire connection for migration %04d: %w", item.version, err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("release migration connection: %w", err))
		}
	}()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return false, fmt.Errorf("begin migration %04d: %w", item.version, err)
	}
	defer func() {
		if resultErr != nil {
			if _, err := conn.ExecContext(context.Background(), "ROLLBACK"); err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("rollback migration %04d: %w", item.version, err))
			}
		}
	}()

	var storedChecksum string
	err = conn.QueryRowContext(
		ctx,
		"SELECT checksum FROM schema_migrations WHERE version = ?", item.version,
	).Scan(&storedChecksum)
	switch {
	case err == nil:
		if storedChecksum != item.checksum {
			return false, fmt.Errorf("applied migration %04d modified", item.version)
		}
		if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
			return false, fmt.Errorf("commit migration check %04d: %w", item.version, err)
		}
		return false, nil
	case !errors.Is(err, sql.ErrNoRows):
		return false, fmt.Errorf("check migration %04d: %w", item.version, err)
	}

	if _, err := conn.ExecContext(ctx, string(item.contents)); err != nil {
		return false, fmt.Errorf("execute migration %04d: %w", item.version, err)
	}
	if _, err := conn.ExecContext(
		ctx,
		"INSERT INTO schema_migrations(version, checksum, applied_at) VALUES (?, ?, ?)",
		item.version, item.checksum, time.Now().UTC(),
	); err != nil {
		return false, fmt.Errorf("record migration %04d: %w", item.version, err)
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return false, fmt.Errorf("commit migration %04d: %w", item.version, err)
	}
	return true, nil
}
