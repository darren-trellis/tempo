package view

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/galaxy-io/tempo/internal/config"
)

const commandHistoryMax = 200

type commandHistory struct {
	items []string
	index int
	draft string
}

func newCommandHistory(items []string) *commandHistory {
	copied := append([]string(nil), items...)
	return &commandHistory{items: copied, index: len(copied)}
}

func (h *commandHistory) browsing() bool {
	return h != nil && h.index < len(h.items)
}

func (h *commandHistory) add(cmd string) {
	if h == nil {
		return
	}
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		h.resetBrowse()
		return
	}
	if n := len(h.items); n > 0 && h.items[n-1] == cmd {
		h.resetBrowse()
		return
	}
	h.items = append(h.items, cmd)
	if len(h.items) > commandHistoryMax {
		h.items = append([]string(nil), h.items[len(h.items)-commandHistoryMax:]...)
	}
	h.resetBrowse()
}

func (h *commandHistory) prev(current string) string {
	if h == nil || len(h.items) == 0 {
		return current
	}
	if !h.browsing() {
		h.draft = current
		h.index = len(h.items)
	}
	if h.index > 0 {
		h.index--
	}
	return h.items[h.index]
}

func (h *commandHistory) next(current string) string {
	if h == nil || !h.browsing() {
		return current
	}
	h.index++
	if h.index >= len(h.items) {
		h.index = len(h.items)
		return h.draft
	}
	return h.items[h.index]
}

func (h *commandHistory) resetBrowse() {
	if h == nil {
		return
	}
	h.index = len(h.items)
	h.draft = ""
}

func loadCommandHistory(path string) *commandHistory {
	h := newCommandHistory(nil)
	if path == "" {
		return h
	}
	f, err := os.Open(path)
	if err != nil {
		return h
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		h.add(line)
	}
	return h
}

func saveCommandHistory(path string, items []string) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	for _, item := range items {
		if item == "" || strings.ContainsAny(item, "\n\r") {
			continue
		}
		b.WriteString(item)
		b.WriteByte('\n')
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "history-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	_, writeErr := tmp.WriteString(b.String())
	closeErr := tmp.Close()
	if writeErr != nil {
		os.Remove(tmpName)
		return writeErr
	}
	if closeErr != nil {
		os.Remove(tmpName)
		return closeErr
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

func (a *App) recordCommand(text string) {
	if a == nil {
		return
	}
	p := a.prompt()
	if p.history == nil {
		p.history = newCommandHistory(nil)
	}
	p.history.add(text)
	_ = saveCommandHistory(config.HistoryPath(), p.history.items)
}

func (a *App) recordSearch(text string) {
	if a == nil {
		return
	}
	p := a.prompt()
	if p.searchHistory == nil {
		p.searchHistory = newCommandHistory(nil)
	}
	p.searchHistory.add(text)
	_ = saveCommandHistory(config.SearchHistoryPath(), p.searchHistory.items)
}
