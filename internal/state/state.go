package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

type Entry struct {
	Version      string `json:"version"`
	URL          string `json:"url,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
	FailingSince string `json:"failing_since,omitempty"`
}

func (e *Entry) UnmarshalJSON(data []byte) error {
	var legacy string
	if err := json.Unmarshal(data, &legacy); err == nil {
		*e = Entry{Version: legacy}
		return nil
	}
	type plain Entry
	return json.Unmarshal(data, (*plain)(e))
}

type State map[string]*Entry

func Load(path string) (State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return State{}, nil
	}
	s := State{}
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return s, nil
}

func (s State) Prune(keep map[string]bool) {
	for name := range s {
		if !keep[name] {
			delete(s, name)
		}
	}
}

func (s State) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
