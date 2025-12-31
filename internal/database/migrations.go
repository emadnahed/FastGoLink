package database

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Migration represents a database migration.
type Migration struct {
	Version   int
	Name      string
	UpSQL     string
	DownSQL   string
	AppliedAt *time.Time
}

// Migrator handles database migrations.
type Migrator struct {
	pool       *Pool
	migrations []Migration
}

// MigrationRecord represents a migration record in the database.
type MigrationRecord struct {
	Version   int
	Name      string
	AppliedAt time.Time
}

// NewMigrator creates a new Migrator with embedded migrations.
func NewMigrator(pool *Pool, migrationsFS embed.FS, dir string) (*Migrator, error) {
	migrations, err := loadMigrations(migrationsFS, dir)
	if err != nil {
		return nil, fmt.Errorf("failed to load migrations: %w", err)
	}

	return &Migrator{
		pool:       pool,
		migrations: migrations,
	}, nil
}

// NewMigratorWithMigrations creates a Migrator with provided migrations.
func NewMigratorWithMigrations(pool *Pool, migrations []Migration) *Migrator {
	return &Migrator{
		pool:       pool,
		migrations: migrations,
	}
}

// loadMigrations loads migrations from an embedded filesystem.
func loadMigrations(migrationsFS embed.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(migrationsFS, dir)
	if err != nil {
		return nil, err
	}

	migrationMap := make(map[int]*Migration)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		// Parse filename: 001_create_urls_table.up.sql or 001_create_urls_table.down.sql
		parts := strings.SplitN(name, "_", 2)
		if len(parts) < 2 {
			continue
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		content, err := fs.ReadFile(migrationsFS, dir+"/"+name)
		if err != nil {
			return nil, fmt.Errorf("failed to read migration %s: %w", name, err)
		}

		if _, exists := migrationMap[version]; !exists {
			migrationMap[version] = &Migration{Version: version}
		}

		// Extract migration name (without version and direction)
		nameParts := strings.Split(parts[1], ".")
		if len(nameParts) >= 2 {
			migrationMap[version].Name = nameParts[0]
			direction := nameParts[len(nameParts)-2]
			if direction == "up" {
				migrationMap[version].UpSQL = string(content)
			} else if direction == "down" {
				migrationMap[version].DownSQL = string(content)
			}
		}
	}

	// Convert map to sorted slice
	migrations := make([]Migration, 0, len(migrationMap))
	for _, m := range migrationMap {
		migrations = append(migrations, *m)
	}
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// EnsureMigrationsTable creates the migrations tracking table if it doesn't exist.
func (m *Migrator) EnsureMigrationsTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			applied_at TIMESTAMPTZ DEFAULT NOW()
		)
	`
	_, err := m.pool.Exec(ctx, query)
	return err
}

// AppliedMigrations returns the list of applied migrations.
func (m *Migrator) AppliedMigrations(ctx context.Context) ([]MigrationRecord, error) {
	query := `SELECT version, name, applied_at FROM schema_migrations ORDER BY version`
	rows, err := m.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []MigrationRecord
	for rows.Next() {
		var r MigrationRecord
		if err := rows.Scan(&r.Version, &r.Name, &r.AppliedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}

	return records, rows.Err()
}

// PendingMigrations returns migrations that haven't been applied yet.
func (m *Migrator) PendingMigrations(ctx context.Context) ([]Migration, error) {
	applied, err := m.AppliedMigrations(ctx)
	if err != nil {
		return nil, err
	}

	appliedSet := make(map[int]bool)
	for _, r := range applied {
		appliedSet[r.Version] = true
	}

	var pending []Migration
	for _, migration := range m.migrations {
		if !appliedSet[migration.Version] {
			pending = append(pending, migration)
		}
	}

	return pending, nil
}

// Up applies all pending migrations.
func (m *Migrator) Up(ctx context.Context) (int, error) {
	if err := m.EnsureMigrationsTable(ctx); err != nil {
		return 0, fmt.Errorf("failed to ensure migrations table: %w", err)
	}

	pending, err := m.PendingMigrations(ctx)
	if err != nil {
		return 0, err
	}

	for _, migration := range pending {
		if err := m.applyMigration(ctx, migration); err != nil {
			return 0, fmt.Errorf("failed to apply migration %d (%s): %w", migration.Version, migration.Name, err)
		}
	}

	return len(pending), nil
}

// Down rolls back the last migration.
func (m *Migrator) Down(ctx context.Context) error {
	applied, err := m.AppliedMigrations(ctx)
	if err != nil {
		return err
	}

	if len(applied) == 0 {
		return nil // Nothing to rollback
	}

	// Find the last applied migration
	lastApplied := applied[len(applied)-1]

	// Find the migration with the matching version
	var migration *Migration
	for i := range m.migrations {
		if m.migrations[i].Version == lastApplied.Version {
			migration = &m.migrations[i]
			break
		}
	}

	if migration == nil {
		return fmt.Errorf("migration %d not found", lastApplied.Version)
	}

	return m.rollbackMigration(ctx, *migration)
}

// applyMigration applies a single migration.
func (m *Migrator) applyMigration(ctx context.Context, migration Migration) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Execute the up SQL
	if _, err := tx.Exec(ctx, migration.UpSQL); err != nil {
		return fmt.Errorf("failed to execute up SQL: %w", err)
	}

	// Record the migration
	_, err = tx.Exec(ctx,
		`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`,
		migration.Version, migration.Name)
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	return tx.Commit(ctx)
}

// rollbackMigration rolls back a single migration.
func (m *Migrator) rollbackMigration(ctx context.Context, migration Migration) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Execute the down SQL
	if migration.DownSQL != "" {
		if _, err := tx.Exec(ctx, migration.DownSQL); err != nil {
			return fmt.Errorf("failed to execute down SQL: %w", err)
		}
	}

	// Remove the migration record
	_, err = tx.Exec(ctx,
		`DELETE FROM schema_migrations WHERE version = $1`,
		migration.Version)
	if err != nil {
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	return tx.Commit(ctx)
}

// CurrentVersion returns the current migration version.
func (m *Migrator) CurrentVersion(ctx context.Context) (int, error) {
	applied, err := m.AppliedMigrations(ctx)
	if err != nil {
		return 0, err
	}

	if len(applied) == 0 {
		return 0, nil
	}

	return applied[len(applied)-1].Version, nil
}
