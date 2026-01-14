package stored

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/thenexusengine/tne_springwire/pkg/logger"
)

// FilesystemFetcher implements Fetcher using the local filesystem
// This is useful for development, testing, and simple deployments
type FilesystemFetcher struct {
	baseDir string
	mu      sync.RWMutex
}

// FilesystemConfig configures the filesystem fetcher
type FilesystemConfig struct {
	// BaseDir is the root directory for stored data
	BaseDir string
}

// NewFilesystemFetcher creates a new filesystem-backed fetcher
func NewFilesystemFetcher(config FilesystemConfig) (*FilesystemFetcher, error) {
	// Ensure base directory exists
	if err := os.MkdirAll(config.BaseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	// Create subdirectories
	subdirs := []string{"requests", "impressions", "responses", "accounts"}
	for _, subdir := range subdirs {
		dir := filepath.Join(config.BaseDir, subdir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create %s directory: %w", subdir, err)
		}
	}

	return &FilesystemFetcher{
		baseDir: config.BaseDir,
	}, nil
}

// FetchRequests retrieves stored request data from the filesystem
func (f *FilesystemFetcher) FetchRequests(ctx context.Context, requestIDs []string) (map[string]json.RawMessage, []error) {
	return f.fetchFromDir(ctx, "requests", requestIDs)
}

// FetchImpressions retrieves stored impression data from the filesystem
func (f *FilesystemFetcher) FetchImpressions(ctx context.Context, impIDs []string) (map[string]json.RawMessage, []error) {
	return f.fetchFromDir(ctx, "impressions", impIDs)
}

// FetchResponses retrieves stored response data from the filesystem
func (f *FilesystemFetcher) FetchResponses(ctx context.Context, respIDs []string) (map[string]json.RawMessage, []error) {
	return f.fetchFromDir(ctx, "responses", respIDs)
}

// FetchAccount retrieves stored account configuration from the filesystem
func (f *FilesystemFetcher) FetchAccount(ctx context.Context, accountID string) (json.RawMessage, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	filePath := filepath.Join(f.baseDir, "accounts", accountID+".json")
	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return json.RawMessage(data), nil
}

// Close releases resources
func (f *FilesystemFetcher) Close() error {
	return nil
}

// fetchFromDir fetches multiple files from a subdirectory
func (f *FilesystemFetcher) fetchFromDir(ctx context.Context, subdir string, ids []string) (map[string]json.RawMessage, []error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := make(map[string]json.RawMessage)
	var errs []error

	dir := filepath.Join(f.baseDir, subdir)

	for _, id := range ids {
		select {
		case <-ctx.Done():
			errs = append(errs, ctx.Err())
			return result, errs
		default:
		}

		filePath := filepath.Join(dir, id+".json")
		data, err := os.ReadFile(filePath)
		if os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("%w: %s", ErrNotFound, id))
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to read %s: %w", id, err))
			continue
		}

		// Validate JSON
		if !json.Valid(data) {
			errs = append(errs, fmt.Errorf("%w: %s", ErrInvalidJSON, id))
			continue
		}

		result[id] = json.RawMessage(data)
	}

	return result, errs
}

// SaveRequest stores a request to the filesystem
func (f *FilesystemFetcher) SaveRequest(id string, data json.RawMessage) error {
	return f.saveToDir("requests", id, data)
}

// SaveImpression stores an impression to the filesystem
func (f *FilesystemFetcher) SaveImpression(id string, data json.RawMessage) error {
	return f.saveToDir("impressions", id, data)
}

// SaveResponse stores a response to the filesystem
func (f *FilesystemFetcher) SaveResponse(id string, data json.RawMessage) error {
	return f.saveToDir("responses", id, data)
}

// SaveAccount stores an account to the filesystem
func (f *FilesystemFetcher) SaveAccount(id string, data json.RawMessage) error {
	return f.saveToDir("accounts", id, data)
}

// saveToDir saves data to a file in a subdirectory
func (f *FilesystemFetcher) saveToDir(subdir, id string, data json.RawMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Validate JSON
	if !json.Valid(data) {
		return ErrInvalidJSON
	}

	filePath := filepath.Join(f.baseDir, subdir, id+".json")

	// Pretty-print for readability
	var prettyData []byte
	var temp interface{}
	if err := json.Unmarshal(data, &temp); err == nil {
		prettyData, _ = json.MarshalIndent(temp, "", "  ")
	} else {
		prettyData = data
	}

	if err := os.WriteFile(filePath, prettyData, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", id, err)
	}

	logger.Log.Debug().
		Str("id", id).
		Str("subdir", subdir).
		Msg("Saved stored data to filesystem")

	return nil
}

// Delete removes a stored file
func (f *FilesystemFetcher) Delete(dataType DataType, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	var subdir string
	switch dataType {
	case DataTypeRequest:
		subdir = "requests"
	case DataTypeImpression:
		subdir = "impressions"
	case DataTypeResponse:
		subdir = "responses"
	case DataTypeAccount:
		subdir = "accounts"
	default:
		return fmt.Errorf("unknown data type: %s", dataType)
	}

	filePath := filepath.Join(f.baseDir, subdir, id+".json")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// ListRequests returns all stored request IDs
func (f *FilesystemFetcher) ListRequests() ([]string, error) {
	return f.listDir("requests")
}

// ListImpressions returns all stored impression IDs
func (f *FilesystemFetcher) ListImpressions() ([]string, error) {
	return f.listDir("impressions")
}

// ListResponses returns all stored response IDs
func (f *FilesystemFetcher) ListResponses() ([]string, error) {
	return f.listDir("responses")
}

// ListAccounts returns all stored account IDs
func (f *FilesystemFetcher) ListAccounts() ([]string, error) {
	return f.listDir("accounts")
}

// listDir returns all IDs (file names without .json) in a subdirectory
func (f *FilesystemFetcher) listDir(subdir string) ([]string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	dir := filepath.Join(f.baseDir, subdir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) == ".json" {
			ids = append(ids, name[:len(name)-5]) // Remove .json extension
		}
	}

	return ids, nil
}

// LoadAll loads all stored data into memory for quick access
// Returns a map of type -> id -> data
func (f *FilesystemFetcher) LoadAll(ctx context.Context) (map[DataType]map[string]json.RawMessage, error) {
	result := map[DataType]map[string]json.RawMessage{
		DataTypeRequest:    make(map[string]json.RawMessage),
		DataTypeImpression: make(map[string]json.RawMessage),
		DataTypeResponse:   make(map[string]json.RawMessage),
		DataTypeAccount:    make(map[string]json.RawMessage),
	}

	subdirs := map[DataType]string{
		DataTypeRequest:    "requests",
		DataTypeImpression: "impressions",
		DataTypeResponse:   "responses",
		DataTypeAccount:    "accounts",
	}

	for dataType, subdir := range subdirs {
		ids, err := f.listDir(subdir)
		if err != nil {
			continue
		}

		data, _ := f.fetchFromDir(ctx, subdir, ids)
		result[dataType] = data
	}

	return result, nil
}
