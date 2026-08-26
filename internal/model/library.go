// Package model is the local library store: imported books, their files and
// per-book reading progress.
package model

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

	"lnreadertui/internal/epub"
)

// Progress is the per-book reading position: chapter index plus a 0..1
// offset inside that chapter (resume restores the viewport).
type Progress struct {
	ChapterIndex int     `json:"chapterIndex"`
	Offset       float64 `json:"offset"`
	ChapterTitle string  `json:"chapterTitle,omitempty"`
	LastReadAt   string  `json:"lastReadAt"`
}

// Book is one library entry.
type Book struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Author    string   `json:"author"`
	File      string   `json:"file"` // path relative to the data dir
	FileType  string   `json:"fileType"`
	Chapters  int      `json:"chapters"`
	AddedAt   string   `json:"addedAt"`
	SourceID  string   `json:"sourceId"`  // bilinovel novel id when downloaded
	SourceURL string   `json:"sourceUrl"` // search result url
	Progress  Progress `json:"progress"`
}

// Store persists the library index at <dataDir>/library.json and imported
// book files under <dataDir>/books/.
type Store struct {
	mu      sync.RWMutex
	dataDir string
	path    string
	books   []*Book
}

// Open loads (or initializes) the library store in dataDir.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dataDir, "books"), 0o755); err != nil {
		return nil, err
	}
	s := &Store{dataDir: dataDir, path: filepath.Join(dataDir, "library.json")}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	var env struct {
		Books []*Book `json:"books"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("library.json: %w", err)
	}
	s.books = env.Books
	return s, nil
}

// Books returns a sorted snapshot of all books.
func (s *Store) Books() []*Book {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Book, len(s.books))
	copy(out, s.books)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Progress.LastReadAt != out[j].Progress.LastReadAt {
			return out[i].Progress.LastReadAt > out[j].Progress.LastReadAt
		}
		return out[i].Title < out[j].Title
	})
	return out
}

// Find locates a book by id.
func (s *Store) Find(id string) *Book {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, b := range s.books {
		if b.ID == id {
			return b
		}
	}
	return nil
}

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(struct {
		Books []*Book `json:"books"`
	}{Books: s.books}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// ImportFile copies src (an .epub/.txt file or a directory containing them)
// into the library. Returns the imported books.
func (s *Store) ImportFile(src string) ([]*Book, error) {
	fi, err := os.Stat(src)
	if err != nil {
		return nil, err
	}
	var files []string
	if fi.IsDir() {
		entries, err := os.ReadDir(src)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if ext == ".epub" || ext == ".txt" {
				files = append(files, filepath.Join(src, e.Name()))
			}
		}
	} else {
		files = []string{src}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .epub/.txt files found")
	}
	books := make([]*Book, 0, len(files))
	for _, f := range files {
		b, err := s.importOne(f)
		if err != nil {
			return nil, err
		}
		books = append(books, b)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.books = append(s.books, books...)
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return books, nil
}

// importOne parses src, copies the file into the library, and returns the
// resulting Book without modifying the index.
func (s *Store) importOne(src string) (*Book, error) {
	parsed, err := epub.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(src), err)
	}
	ext := strings.ToLower(filepath.Ext(src))
	id := newID()
	dest := filepath.Join(s.dataDir, "books", id+ext)
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return nil, err
	}
	author := parsed.Author
	if author == "" {
		author = "Unknown"
	}
	return &Book{
		ID:       id,
		Title:    parsed.Title,
		Author:   author,
		File:     filepath.Join("books", id+ext),
		FileType: strings.TrimPrefix(ext, "."),
		Chapters: len(parsed.Chapters),
		AddedAt:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// Delete removes a book and its file.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i, b := range s.books {
		if b.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	b := s.books[idx]
	s.books = append(s.books[:idx], s.books[idx+1:]...)
	// Ignore file-removal errors (e.g. missing file); keep index consistent.
	_ = os.Remove(filepath.Join(s.dataDir, b.File))
	return s.saveLocked()
}

// SetProgress records reading progress for a book (chapterIndex zero-based,
// offset 0..1 within the chapter, title of the current chapter).
func (s *Store) SetProgress(id string, chapterIndex int, offset float64, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range s.books {
		if b.ID == id {
			if chapterIndex < 0 {
				chapterIndex = 0
			}
			if offset < 0 {
				offset = 0
			}
			if offset > 1 {
				offset = 1
			}
			b.Progress = Progress{
				ChapterIndex: chapterIndex,
				Offset:       offset,
				ChapterTitle: title,
				LastReadAt:   time.Now().UTC().Format(time.RFC3339),
			}
			return s.saveLocked()
		}
	}
	return fmt.Errorf("book not found: %s", id)
}

// ResetProgress clears the reading progress of a book (marks it unread).
func (s *Store) ResetProgress(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range s.books {
		if b.ID == id {
			b.Progress = Progress{}
			return s.saveLocked()
		}
	}
	return fmt.Errorf("book not found: %s", id)
}

// RegisterDownloaded adds an EPUB that was downloaded and already written
// to srcPath (kept in place; not copied again).
func (s *Store) RegisterDownloaded(srcPath, title, author, novelID, srcURL string, totalChapters int) (*Book, error) {
	id := newID()
	ext := strings.ToLower(filepath.Ext(srcPath))
	finalPath := filepath.Join("books", id+ext)
	if err := copyFile(srcPath, filepath.Join(s.dataDir, finalPath)); err != nil {
		return nil, err
	}
	b := &Book{
		ID:        id,
		Title:     title,
		Author:    author,
		File:      finalPath,
		FileType:  strings.TrimPrefix(ext, "."),
		Chapters:  totalChapters,
		AddedAt:   time.Now().UTC().Format(time.RFC3339),
		SourceID:  novelID,
		SourceURL: srcURL,
	}
	// Parse once (from the destination) so metadata/title/chapter count
	// match the actual file; prefer the downloaded metadata when parsing
	// yields nothing better.
	if parsed, err := epub.ReadFile(filepath.Join(s.dataDir, finalPath)); err == nil {
		if parsed.Title != "" {
			b.Title = parsed.Title
		}
		if parsed.Author != "" && parsed.Author != "Unknown" {
			b.Author = parsed.Author
		}
		if parsed.Chapters != nil && len(parsed.Chapters) > 0 {
			b.Chapters = len(parsed.Chapters)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.books = append(s.books, b)
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return b, nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// FilePath returns the absolute path of a book file.
func (s *Store) FilePath(b *Book) string { return filepath.Join(s.dataDir, b.File) }

func newID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
