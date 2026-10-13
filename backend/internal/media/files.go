package media

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"regexp"
)

var storageKeyPattern = regexp.MustCompile(`^[a-f0-9]{32}\.(png|jpg|mp4|webm)$`)

// FileStore anchors every operation to one open directory descriptor. Public
// names and database IDs never become filesystem paths.
type FileStore struct{ root *os.Root }

func NewFileStore(directory string) (*FileStore, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	return &FileStore{root: root}, nil
}
func (s *FileStore) Close() error { return s.root.Close() }
func (s *FileStore) Open(key string) (*os.File, error) {
	if !storageKeyPattern.MatchString(key) {
		return nil, ErrNotFound
	}
	before, err := s.root.Lstat(key)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, ErrNotFound
	}
	f, err := s.root.Open(key)
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		f.Close()
		if err != nil {
			return nil, err
		}
		return nil, ErrNotFound
	}
	return f, nil
}
func (s *FileStore) Remove(key string) error {
	if !storageKeyPattern.MatchString(key) {
		return ErrInvalidInput
	}
	return s.root.Remove(key)
}

type temporary struct {
	file   *os.File
	key    string
	size   int64
	digest []byte
}

func (s *FileStore) stage(ctx context.Context, reader io.Reader, maximum int64) (temporary, error) {
	var t temporary
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return t, err
	}
	t.key = ".upload-" + hex.EncodeToString(random)
	f, err := s.root.OpenFile(t.key, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return t, err
	}
	t.file = f
	ok := false
	defer func() {
		if !ok {
			f.Close()
			_ = s.root.Remove(t.key)
		}
	}()
	hash := sha256.New()
	// io.CopyBuffer keeps videos bounded in memory. The extra byte detects an
	// over-limit stream even when the client omits Content-Length.
	n, err := io.CopyBuffer(io.MultiWriter(f, hash), io.LimitReader(contextReader{ctx, reader}, maximum+1), make([]byte, 32<<10))
	if err != nil {
		return temporary{}, err
	}
	if n > maximum {
		return temporary{}, ErrTooLarge
	}
	if n == 0 {
		return temporary{}, ErrInvalidFile
	}
	if err := ctx.Err(); err != nil {
		return temporary{}, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return temporary{}, err
	}
	t.size = n
	t.digest = hash.Sum(nil)
	ok = true
	return t, nil
}
func (s *FileStore) discard(t temporary) {
	if t.file != nil {
		_ = t.file.Close()
	}
	_ = s.root.Remove(t.key)
}
func (s *FileStore) finalize(ctx context.Context, t temporary, extension string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := t.file.Sync(); err != nil {
		return "", err
	}
	if err := t.file.Close(); err != nil {
		return "", err
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	key := hex.EncodeToString(random) + "." + extension
	if !storageKeyPattern.MatchString(key) {
		return "", ErrInvalidFile
	}
	if err := s.root.Rename(t.key, key); err != nil {
		return "", err
	}
	return key, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(b)
	if canceled := r.ctx.Err(); canceled != nil {
		return n, canceled
	}
	return n, err
}

func isMissing(err error) bool { return errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrNotFound) }
