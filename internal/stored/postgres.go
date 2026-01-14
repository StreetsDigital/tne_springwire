package stored

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/thenexusengine/tne_springwire/pkg/logger"
)

// PostgresFetcher implements Fetcher using PostgreSQL as the backend
type PostgresFetcher struct {
	db     *sql.DB
	config PostgresConfig
}

// PostgresConfig configures the PostgreSQL fetcher
type PostgresConfig struct {
	// RequestsTable is the table name for stored requests
	RequestsTable string
	// ImpressionsTable is the table name for stored impressions
	ImpressionsTable string
	// ResponsesTable is the table name for stored responses
	ResponsesTable string
	// AccountsTable is the table name for stored accounts
	AccountsTable string
	// QueryTimeout is the timeout for database queries
	QueryTimeout time.Duration
}

// DefaultPostgresConfig returns sensible defaults
func DefaultPostgresConfig() PostgresConfig {
	return PostgresConfig{
		RequestsTable:    "stored_requests",
		ImpressionsTable: "stored_impressions",
		ResponsesTable:   "stored_responses",
		AccountsTable:    "stored_accounts",
		QueryTimeout:     5 * time.Second,
	}
}

// NewPostgresFetcher creates a new PostgreSQL-backed fetcher
func NewPostgresFetcher(db *sql.DB, config PostgresConfig) *PostgresFetcher {
	return &PostgresFetcher{
		db:     db,
		config: config,
	}
}

// FetchRequests retrieves stored request data from PostgreSQL
func (f *PostgresFetcher) FetchRequests(ctx context.Context, requestIDs []string) (map[string]json.RawMessage, []error) {
	return f.fetchByIDs(ctx, f.config.RequestsTable, requestIDs)
}

// FetchImpressions retrieves stored impression data from PostgreSQL
func (f *PostgresFetcher) FetchImpressions(ctx context.Context, impIDs []string) (map[string]json.RawMessage, []error) {
	return f.fetchByIDs(ctx, f.config.ImpressionsTable, impIDs)
}

// FetchResponses retrieves stored response data from PostgreSQL
func (f *PostgresFetcher) FetchResponses(ctx context.Context, respIDs []string) (map[string]json.RawMessage, []error) {
	return f.fetchByIDs(ctx, f.config.ResponsesTable, respIDs)
}

// FetchAccount retrieves stored account configuration from PostgreSQL
func (f *PostgresFetcher) FetchAccount(ctx context.Context, accountID string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, f.config.QueryTimeout)
	defer cancel()

	query := fmt.Sprintf(
		"SELECT data FROM %s WHERE id = $1 AND (disabled IS NULL OR disabled = false)",
		f.config.AccountsTable,
	)

	var data json.RawMessage
	err := f.db.QueryRowContext(ctx, query, accountID).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		logger.Log.Error().
			Err(err).
			Str("account_id", accountID).
			Msg("Failed to fetch account from PostgreSQL")
		return nil, err
	}

	return data, nil
}

// Close releases resources (the db connection is managed externally)
func (f *PostgresFetcher) Close() error {
	// We don't own the db connection, so don't close it
	return nil
}

// fetchByIDs is a generic method to fetch multiple records by ID
func (f *PostgresFetcher) fetchByIDs(ctx context.Context, table string, ids []string) (map[string]json.RawMessage, []error) {
	if len(ids) == 0 {
		return make(map[string]json.RawMessage), nil
	}

	ctx, cancel := context.WithTimeout(ctx, f.config.QueryTimeout)
	defer cancel()

	// Build parameterized query
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(
		"SELECT id, data FROM %s WHERE id IN (%s) AND (disabled IS NULL OR disabled = false)",
		table,
		strings.Join(placeholders, ", "),
	)

	rows, err := f.db.QueryContext(ctx, query, args...)
	if err != nil {
		logger.Log.Error().
			Err(err).
			Str("table", table).
			Int("count", len(ids)).
			Msg("Failed to fetch from PostgreSQL")
		return nil, []error{err}
	}
	defer rows.Close()

	result := make(map[string]json.RawMessage)
	var errs []error

	for rows.Next() {
		var id string
		var data json.RawMessage
		if err := rows.Scan(&id, &data); err != nil {
			errs = append(errs, fmt.Errorf("scan error for %s: %w", table, err))
			continue
		}
		result[id] = data
	}

	if err := rows.Err(); err != nil {
		errs = append(errs, err)
	}

	// Check for missing IDs
	for _, id := range ids {
		if _, found := result[id]; !found {
			errs = append(errs, fmt.Errorf("%w: %s in %s", ErrNotFound, id, table))
		}
	}

	return result, errs
}

// CreateTables creates the necessary tables if they don't exist
func (f *PostgresFetcher) CreateTables(ctx context.Context) error {
	tables := []struct {
		name   string
		schema string
	}{
		{
			name: f.config.RequestsTable,
			schema: `
				CREATE TABLE IF NOT EXISTS %s (
					id VARCHAR(255) PRIMARY KEY,
					data JSONB NOT NULL,
					account_id VARCHAR(255),
					disabled BOOLEAN DEFAULT false,
					created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
					updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
				)`,
		},
		{
			name: f.config.ImpressionsTable,
			schema: `
				CREATE TABLE IF NOT EXISTS %s (
					id VARCHAR(255) PRIMARY KEY,
					data JSONB NOT NULL,
					account_id VARCHAR(255),
					disabled BOOLEAN DEFAULT false,
					created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
					updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
				)`,
		},
		{
			name: f.config.ResponsesTable,
			schema: `
				CREATE TABLE IF NOT EXISTS %s (
					id VARCHAR(255) PRIMARY KEY,
					data JSONB NOT NULL,
					account_id VARCHAR(255),
					disabled BOOLEAN DEFAULT false,
					created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
					updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
				)`,
		},
		{
			name: f.config.AccountsTable,
			schema: `
				CREATE TABLE IF NOT EXISTS %s (
					id VARCHAR(255) PRIMARY KEY,
					data JSONB NOT NULL,
					disabled BOOLEAN DEFAULT false,
					created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
					updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
				)`,
		},
	}

	for _, table := range tables {
		query := fmt.Sprintf(table.schema, table.name)
		if _, err := f.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed to create table %s: %w", table.name, err)
		}

		// Create index on account_id for faster lookups
		if table.name != f.config.AccountsTable {
			indexQuery := fmt.Sprintf(
				"CREATE INDEX IF NOT EXISTS idx_%s_account_id ON %s(account_id)",
				table.name, table.name,
			)
			if _, err := f.db.ExecContext(ctx, indexQuery); err != nil {
				logger.Log.Warn().
					Err(err).
					Str("table", table.name).
					Msg("Failed to create account_id index")
			}
		}
	}

	return nil
}

// SaveRequest stores or updates a request
func (f *PostgresFetcher) SaveRequest(ctx context.Context, id string, data json.RawMessage, accountID string) error {
	return f.save(ctx, f.config.RequestsTable, id, data, accountID)
}

// SaveImpression stores or updates an impression
func (f *PostgresFetcher) SaveImpression(ctx context.Context, id string, data json.RawMessage, accountID string) error {
	return f.save(ctx, f.config.ImpressionsTable, id, data, accountID)
}

// SaveResponse stores or updates a response
func (f *PostgresFetcher) SaveResponse(ctx context.Context, id string, data json.RawMessage, accountID string) error {
	return f.save(ctx, f.config.ResponsesTable, id, data, accountID)
}

// SaveAccount stores or updates an account
func (f *PostgresFetcher) SaveAccount(ctx context.Context, id string, data json.RawMessage) error {
	ctx, cancel := context.WithTimeout(ctx, f.config.QueryTimeout)
	defer cancel()

	query := fmt.Sprintf(`
		INSERT INTO %s (id, data, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (id) DO UPDATE SET
			data = EXCLUDED.data,
			updated_at = NOW()
	`, f.config.AccountsTable)

	_, err := f.db.ExecContext(ctx, query, id, data)
	return err
}

// save is a generic method to store data
func (f *PostgresFetcher) save(ctx context.Context, table, id string, data json.RawMessage, accountID string) error {
	ctx, cancel := context.WithTimeout(ctx, f.config.QueryTimeout)
	defer cancel()

	query := fmt.Sprintf(`
		INSERT INTO %s (id, data, account_id, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (id) DO UPDATE SET
			data = EXCLUDED.data,
			account_id = EXCLUDED.account_id,
			updated_at = NOW()
	`, table)

	_, err := f.db.ExecContext(ctx, query, id, data, accountID)
	return err
}

// Delete removes a stored item
func (f *PostgresFetcher) Delete(ctx context.Context, dataType DataType, id string) error {
	ctx, cancel := context.WithTimeout(ctx, f.config.QueryTimeout)
	defer cancel()

	var table string
	switch dataType {
	case DataTypeRequest:
		table = f.config.RequestsTable
	case DataTypeImpression:
		table = f.config.ImpressionsTable
	case DataTypeResponse:
		table = f.config.ResponsesTable
	case DataTypeAccount:
		table = f.config.AccountsTable
	default:
		return fmt.Errorf("unknown data type: %s", dataType)
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", table)
	_, err := f.db.ExecContext(ctx, query, id)
	return err
}

// Disable soft-deletes a stored item
func (f *PostgresFetcher) Disable(ctx context.Context, dataType DataType, id string) error {
	ctx, cancel := context.WithTimeout(ctx, f.config.QueryTimeout)
	defer cancel()

	var table string
	switch dataType {
	case DataTypeRequest:
		table = f.config.RequestsTable
	case DataTypeImpression:
		table = f.config.ImpressionsTable
	case DataTypeResponse:
		table = f.config.ResponsesTable
	case DataTypeAccount:
		table = f.config.AccountsTable
	default:
		return fmt.Errorf("unknown data type: %s", dataType)
	}

	query := fmt.Sprintf("UPDATE %s SET disabled = true, updated_at = NOW() WHERE id = $1", table)
	_, err := f.db.ExecContext(ctx, query, id)
	return err
}

// ListRequests returns all stored request IDs for an account
func (f *PostgresFetcher) ListRequests(ctx context.Context, accountID string) ([]string, error) {
	return f.listIDs(ctx, f.config.RequestsTable, accountID)
}

// ListImpressions returns all stored impression IDs for an account
func (f *PostgresFetcher) ListImpressions(ctx context.Context, accountID string) ([]string, error) {
	return f.listIDs(ctx, f.config.ImpressionsTable, accountID)
}

// listIDs returns all IDs from a table for an account
func (f *PostgresFetcher) listIDs(ctx context.Context, table, accountID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, f.config.QueryTimeout)
	defer cancel()

	var query string
	var rows *sql.Rows
	var err error

	if accountID == "" {
		query = fmt.Sprintf("SELECT id FROM %s WHERE disabled IS NULL OR disabled = false ORDER BY created_at DESC", table)
		rows, err = f.db.QueryContext(ctx, query)
	} else {
		query = fmt.Sprintf("SELECT id FROM %s WHERE account_id = $1 AND (disabled IS NULL OR disabled = false) ORDER BY created_at DESC", table)
		rows, err = f.db.QueryContext(ctx, query, accountID)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}
