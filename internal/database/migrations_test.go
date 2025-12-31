package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrator_Up(t *testing.T) {
	skipIfNoPostgres(t)

	ctx := context.Background()
	cfg := testDBConfig()

	pool, err := NewPool(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()

	// Clean up any existing migrations table
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS test_table")

	migrations := []Migration{
		{
			Version: 1,
			Name:    "create_test_table",
			UpSQL:   "CREATE TABLE test_table (id SERIAL PRIMARY KEY, name VARCHAR(255))",
			DownSQL: "DROP TABLE test_table",
		},
		{
			Version: 2,
			Name:    "add_email_column",
			UpSQL:   "ALTER TABLE test_table ADD COLUMN email VARCHAR(255)",
			DownSQL: "ALTER TABLE test_table DROP COLUMN email",
		},
	}

	migrator := NewMigratorWithMigrations(pool, migrations)

	// Run migrations
	applied, err := migrator.Up(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, applied)

	// Verify current version
	version, err := migrator.CurrentVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, version)

	// Run again - should apply 0
	applied, err = migrator.Up(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, applied)

	// Clean up
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS test_table")
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")
}

func TestMigrator_Down(t *testing.T) {
	skipIfNoPostgres(t)

	ctx := context.Background()
	cfg := testDBConfig()

	pool, err := NewPool(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()

	// Clean up
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS test_table")

	migrations := []Migration{
		{
			Version: 1,
			Name:    "create_test_table",
			UpSQL:   "CREATE TABLE test_table (id SERIAL PRIMARY KEY)",
			DownSQL: "DROP TABLE test_table",
		},
	}

	migrator := NewMigratorWithMigrations(pool, migrations)

	// Apply migration
	_, err = migrator.Up(ctx)
	require.NoError(t, err)

	// Verify table exists
	var exists bool
	err = pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_name = 'test_table'
		)
	`).Scan(&exists)
	require.NoError(t, err)
	assert.True(t, exists)

	// Rollback
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Verify table is gone
	err = pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_name = 'test_table'
		)
	`).Scan(&exists)
	require.NoError(t, err)
	assert.False(t, exists)

	// Clean up
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")
}

func TestMigrator_PendingMigrations(t *testing.T) {
	skipIfNoPostgres(t)

	ctx := context.Background()
	cfg := testDBConfig()

	pool, err := NewPool(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()

	// Clean up
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")

	migrations := []Migration{
		{Version: 1, Name: "first", UpSQL: "SELECT 1", DownSQL: "SELECT 1"},
		{Version: 2, Name: "second", UpSQL: "SELECT 1", DownSQL: "SELECT 1"},
		{Version: 3, Name: "third", UpSQL: "SELECT 1", DownSQL: "SELECT 1"},
	}

	migrator := NewMigratorWithMigrations(pool, migrations)
	require.NoError(t, migrator.EnsureMigrationsTable(ctx))

	// All should be pending
	pending, err := migrator.PendingMigrations(ctx)
	require.NoError(t, err)
	assert.Len(t, pending, 3)

	// Apply first migration manually
	_, err = pool.Exec(ctx, `INSERT INTO schema_migrations (version, name) VALUES (1, 'first')`)
	require.NoError(t, err)

	// Now only 2 should be pending
	pending, err = migrator.PendingMigrations(ctx)
	require.NoError(t, err)
	assert.Len(t, pending, 2)
	assert.Equal(t, 2, pending[0].Version)
	assert.Equal(t, 3, pending[1].Version)

	// Clean up
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")
}

func TestMigrator_AppliedMigrations(t *testing.T) {
	skipIfNoPostgres(t)

	ctx := context.Background()
	cfg := testDBConfig()

	pool, err := NewPool(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()

	// Clean up and setup
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")

	migrator := NewMigratorWithMigrations(pool, nil)
	require.NoError(t, migrator.EnsureMigrationsTable(ctx))

	// Insert some records
	_, err = pool.Exec(ctx, `INSERT INTO schema_migrations (version, name) VALUES (1, 'first'), (2, 'second')`)
	require.NoError(t, err)

	applied, err := migrator.AppliedMigrations(ctx)
	require.NoError(t, err)
	assert.Len(t, applied, 2)
	assert.Equal(t, 1, applied[0].Version)
	assert.Equal(t, "first", applied[0].Name)
	assert.Equal(t, 2, applied[1].Version)
	assert.Equal(t, "second", applied[1].Name)

	// Clean up
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")
}

func TestMigrator_CurrentVersion(t *testing.T) {
	skipIfNoPostgres(t)

	ctx := context.Background()
	cfg := testDBConfig()

	pool, err := NewPool(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()

	// Clean up
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")

	migrator := NewMigratorWithMigrations(pool, nil)
	require.NoError(t, migrator.EnsureMigrationsTable(ctx))

	// Should be 0 with no migrations
	version, err := migrator.CurrentVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, version)

	// Add a migration
	_, err = pool.Exec(ctx, `INSERT INTO schema_migrations (version, name) VALUES (5, 'test')`)
	require.NoError(t, err)

	version, err = migrator.CurrentVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, 5, version)

	// Clean up
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")
}

func TestMigration_Transaction(t *testing.T) {
	skipIfNoPostgres(t)

	ctx := context.Background()
	cfg := testDBConfig()

	pool, err := NewPool(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()

	// Clean up
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS test_tx_table")

	// Create a migration with invalid SQL that will fail
	migrations := []Migration{
		{
			Version: 1,
			Name:    "valid_migration",
			UpSQL:   "CREATE TABLE test_tx_table (id SERIAL PRIMARY KEY)",
			DownSQL: "DROP TABLE test_tx_table",
		},
		{
			Version: 2,
			Name:    "invalid_migration",
			UpSQL:   "THIS IS NOT VALID SQL",
			DownSQL: "",
		},
	}

	migrator := NewMigratorWithMigrations(pool, migrations)

	// This should fail on the second migration
	_, err = migrator.Up(ctx)
	assert.Error(t, err)

	// First migration should have been applied
	version, err := migrator.CurrentVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, version)

	// Clean up
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS test_tx_table")
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")
}

func TestMigrator_EnsureMigrationsTable(t *testing.T) {
	skipIfNoPostgres(t)

	ctx := context.Background()
	cfg := testDBConfig()

	pool, err := NewPool(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()

	// Clean up
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")

	migrator := NewMigratorWithMigrations(pool, nil)

	// Create table
	err = migrator.EnsureMigrationsTable(ctx)
	require.NoError(t, err)

	// Verify table exists
	var exists bool
	err = pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_name = 'schema_migrations'
		)
	`).Scan(&exists)
	require.NoError(t, err)
	assert.True(t, exists)

	// Calling again should be idempotent
	err = migrator.EnsureMigrationsTable(ctx)
	require.NoError(t, err)

	// Clean up
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")
}

func TestMigration_TimestampRecording(t *testing.T) {
	skipIfNoPostgres(t)

	ctx := context.Background()
	cfg := testDBConfig()

	pool, err := NewPool(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()

	// Clean up
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")

	migrations := []Migration{
		{Version: 1, Name: "test", UpSQL: "SELECT 1", DownSQL: "SELECT 1"},
	}

	migrator := NewMigratorWithMigrations(pool, migrations)

	before := time.Now().Add(-1 * time.Second)
	_, err = migrator.Up(ctx)
	require.NoError(t, err)
	after := time.Now().Add(1 * time.Second)

	applied, err := migrator.AppliedMigrations(ctx)
	require.NoError(t, err)
	require.Len(t, applied, 1)

	assert.True(t, applied[0].AppliedAt.After(before))
	assert.True(t, applied[0].AppliedAt.Before(after))

	// Clean up
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")
}
