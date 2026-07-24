package journal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Entry struct {
	SchemaVersion  string    `json:"schema_version"`
	IdempotencyKey string    `json:"idempotency_key"`
	Repository     string    `json:"repository"`
	PullRequest    int       `json:"pull_request"`
	HeadSHA        string    `json:"head_sha"`
	ReviewID       int64     `json:"review_id,omitempty"`
	ReviewNodeID   string    `json:"review_node_id,omitempty"`
	State          string    `json:"state"`
	CommentIDs     []string  `json:"comment_ids,omitempty"`
	ReviewURL      string    `json:"review_url,omitempty"`
	Error          string    `json:"error,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Store struct {
	Dir string
}

func DefaultStore() (*Store, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("resolve cache directory: %w", err)
	}
	return &Store{Dir: filepath.Join(dir, "gh-code-review", "transactions")}, nil
}

func (s *Store) Load(key string) (*Entry, error) {
	data, err := os.ReadFile(s.path(key))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("decode transaction journal: %w", err)
	}
	return &entry, nil
}

func (s *Store) Save(entry *Entry) error {
	if entry.IdempotencyKey == "" {
		return nil
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	entry.SchemaVersion = "1.0"
	entry.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	target := s.path(entry.IdempotencyKey)
	temp, err := os.CreateTemp(s.Dir, ".journal-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, target)
}

func (s *Store) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(s.Dir, hex.EncodeToString(sum[:])+".json")
}
