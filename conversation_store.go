package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxConversationFileBytes = 16 << 20

type ConversationMessage struct {
	Role    string    `json:"role"`
	Content string    `json:"content"`
	At      time.Time `json:"at"`
}

type Conversation struct {
	ID        string                `json:"id"`
	Title     string                `json:"title"`
	CreatedAt time.Time             `json:"createdAt"`
	UpdatedAt time.Time             `json:"updatedAt"`
	Messages  []ConversationMessage `json:"messages"`
}

type ConversationSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type conversationStore struct {
	dir string
	mu  sync.Mutex
}

func newConversationStore() (*conversationStore, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".local", "share", "ubuntu-dev-assistant", "conversations")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create conversation history folder: %w", err)
	}
	return &conversationStore{dir: dir}, nil
}

func newConversation() (*Conversation, error) {
	var idBytes [16]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return nil, err
	}
	now := time.Now()
	return &Conversation{ID: hex.EncodeToString(idBytes[:]), Title: "New chat", CreatedAt: now, UpdatedAt: now, Messages: []ConversationMessage{}}, nil
}

func (s *conversationStore) path(id string) (string, error) {
	if len(id) != 32 {
		return "", fmt.Errorf("invalid conversation id")
	}
	if _, err := hex.DecodeString(id); err != nil {
		return "", fmt.Errorf("invalid conversation id")
	}
	return filepath.Join(s.dir, id+".json"), nil
}

func (s *conversationStore) save(conv *Conversation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(conv.ID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(conv, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > maxConversationFileBytes {
		return fmt.Errorf("conversation is too large to save")
	}
	tmp, err := os.CreateTemp(s.dir, ".conversation-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return nil
}

func (s *conversationStore) load(id string) (*Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(id)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxConversationFileBytes {
		return nil, fmt.Errorf("conversation file is too large")
	}
	var conv Conversation
	dec := json.NewDecoder(f)
	if err := dec.Decode(&conv); err != nil {
		return nil, fmt.Errorf("read conversation: %w", err)
	}
	if conv.ID != id {
		return nil, fmt.Errorf("conversation id does not match file")
	}
	return &conv, nil
}

func (s *conversationStore) list() ([]ConversationSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	out := make([]ConversationSummary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if len(id) != 32 {
			continue
		}
		if _, err := hex.DecodeString(id); err != nil {
			continue
		}
		path := filepath.Join(s.dir, entry.Name())
		info, err := entry.Info()
		if err != nil || info.Size() > maxConversationFileBytes {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var conv Conversation
		if json.Unmarshal(data, &conv) != nil || conv.ID != id || len(conv.Messages) == 0 {
			continue
		}
		out = append(out, ConversationSummary{ID: conv.ID, Title: conv.Title, CreatedAt: conv.CreatedAt, UpdatedAt: conv.UpdatedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (s *conversationStore) delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return nil
}

func titleFor(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > 48 {
		return string(runes[:48]) + "…"
	}
	if text == "" {
		return "New chat"
	}
	return text
}
