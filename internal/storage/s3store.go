package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"

	"recipe-to-reminders/internal/models"
)

const maxFileSizeWarning = 1024 * 1024 // 1MB

// S3Store implements Store using an S3-compatible backend.
type S3Store struct {
	client S3API
	bucket string
	key    string
}

// NewS3Store creates an S3Store targeting the given bucket and object key.
func NewS3Store(client S3API, bucket, key string) *S3Store {
	return &S3Store{client: client, bucket: bucket, key: key}
}

// ReadAll loads the recipe collection from S3. If the object does not exist,
// it returns an empty collection with Version 1 and an empty ETag. If the
// object is corrupt JSON, it attempts to recover from a previous S3 version.
func (s *S3Store) ReadAll(ctx context.Context) (*models.RecipeCollection, string, error) {
	body, etag, err := s.client.GetObject(ctx, s.bucket, s.key)
	if err != nil {
		if err == ErrNotFound {
			return &models.RecipeCollection{Version: 1, Recipes: []models.SavedRecipe{}}, "", nil
		}
		return nil, "", fmt.Errorf("s3 get: %w", err)
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		return nil, "", fmt.Errorf("s3 read body: %w", err)
	}

	if len(data) > maxFileSizeWarning {
		log.Printf("WARNING: recipes.json is %d bytes (>1MB) — consider migrating to DynamoDB", len(data))
	}

	var col models.RecipeCollection
	if err := json.Unmarshal(data, &col); err != nil {
		log.Printf("ERROR: recipes.json corrupt: %v — attempting version recovery", err)
		return s.recoverFromVersion(ctx)
	}

	return &col, etag, nil
}

// WriteAll serializes the collection to JSON and writes it to S3 with an
// optional ETag-based conditional put. Pass an empty etag for unconditional
// writes (e.g., creating the file for the first time).
func (s *S3Store) WriteAll(ctx context.Context, collection *models.RecipeCollection, etag string) error {
	data, err := json.Marshal(collection)
	if err != nil {
		return fmt.Errorf("marshal collection: %w", err)
	}

	if err := s.client.PutObject(ctx, s.bucket, s.key, data, etag); err != nil {
		return err
	}

	return nil
}

// recoverFromVersion attempts to load recipes.json from a previous S3 object
// version after the current version is found to be corrupt.
func (s *S3Store) recoverFromVersion(ctx context.Context) (*models.RecipeCollection, string, error) {
	versions, err := s.client.ListObjectVersions(ctx, s.bucket, s.key, 5)
	if err != nil {
		return nil, "", fmt.Errorf("list versions failed: %w", err)
	}

	for _, v := range versions {
		if v.IsLatest {
			continue
		}
		body, err := s.client.GetObjectVersion(ctx, s.bucket, s.key, v.VersionID)
		if err != nil {
			continue
		}
		defer body.Close()

		data, err := io.ReadAll(body)
		if err != nil {
			continue
		}

		var col models.RecipeCollection
		if err := json.Unmarshal(data, &col); err != nil {
			continue
		}

		log.Printf("RECOVERED: loaded recipes.json from version %s", v.VersionID)
		return &col, "", nil
	}

	return nil, "", fmt.Errorf("recipes.json corrupt and no recoverable version found")
}
