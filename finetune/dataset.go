package finetune

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Dataset holds a collection of training conversations.
type Dataset struct {
	samples []json.RawMessage
}

// LoadDataset loads all JSON files from a directory.
func LoadDataset(dir string) (*Dataset, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("finetune: read dir: %w", err)
	}
	ds := &Dataset{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		ds.samples = append(ds.samples, json.RawMessage(b))
	}
	return ds, nil
}

// Len returns the number of samples.
func (ds *Dataset) Len() int { return len(ds.samples) }

// FilterFunc is a predicate for filtering dataset samples.
type FilterFunc func(raw json.RawMessage) bool

// MinTurns keeps samples with at least n conversation turns.
func MinTurns(n int) FilterFunc {
	return func(raw json.RawMessage) bool {
		var conv struct {
			Conversations []json.RawMessage `json:"conversations"`
			Messages      []json.RawMessage `json:"messages"`
		}
		json.Unmarshal(raw, &conv)
		count := len(conv.Conversations)
		if count == 0 {
			count = len(conv.Messages)
		}
		return count >= n
	}
}

// MaxTurns keeps samples with at most n conversation turns.
func MaxTurns(n int) FilterFunc {
	return func(raw json.RawMessage) bool {
		var conv struct {
			Conversations []json.RawMessage `json:"conversations"`
			Messages      []json.RawMessage `json:"messages"`
		}
		json.Unmarshal(raw, &conv)
		count := len(conv.Conversations)
		if count == 0 {
			count = len(conv.Messages)
		}
		return count <= n
	}
}

// NoErrors keeps samples that have no error indicators.
func NoErrors() FilterFunc {
	return func(raw json.RawMessage) bool {
		return !strings.Contains(string(raw), "\"is_error\":true") &&
			!strings.Contains(string(raw), "Error:")
	}
}

// ContainsTool keeps samples that mention a specific tool.
func ContainsTool(toolName string) FilterFunc {
	return func(raw json.RawMessage) bool {
		return strings.Contains(string(raw), toolName)
	}
}

// Filter returns a new Dataset with only samples matching all filters.
func (ds *Dataset) Filter(filters ...FilterFunc) *Dataset {
	result := &Dataset{}
	for _, s := range ds.samples {
		keep := true
		for _, f := range filters {
			if !f(s) {
				keep = false
				break
			}
		}
		if keep {
			result.samples = append(result.samples, s)
		}
	}
	return result
}

// Split splits the dataset into train and test sets.
func (ds *Dataset) Split(trainRatio float64) (*Dataset, *Dataset) {
	n := int(float64(len(ds.samples)) * trainRatio)
	train := &Dataset{samples: ds.samples[:n]}
	test := &Dataset{samples: ds.samples[n:]}
	return train, test
}

// Export writes the dataset as JSONL to the given path.
func (ds *Dataset) Export(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("finetune: create %s: %w", path, err)
	}
	defer f.Close()
	for _, s := range ds.samples {
		f.Write(s)
		f.Write([]byte("\n"))
	}
	return nil
}

// Samples returns the raw JSON samples.
func (ds *Dataset) Samples() []json.RawMessage {
	return ds.samples
}
