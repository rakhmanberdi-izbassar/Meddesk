package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meddesk/meddesk/backend/internal/auth"
)

const testID = "11111111-1111-4111-8111-111111111111"

func staffUser() auth.User { return auth.User{ID: testID, Role: auth.RoleEngineer, IsActive: true} }

type memoryStore struct {
	created int
	files   map[string]File
	err     error
}

func (m *memoryStore) Create(_ context.Context, _ auth.User, f File) (File, error) {
	m.created++
	if m.err != nil {
		return File{}, m.err
	}
	f.ID = testID
	f.URL = "/api/v1/media/" + f.ID
	f.CreatedAt = time.Now().UTC()
	m.files[f.ID] = f
	return f, nil
}
func (m *memoryStore) Find(_ context.Context, _ auth.User, id string) (File, error) {
	if m.err != nil {
		return File{}, m.err
	}
	f, ok := m.files[id]
	if !ok {
		return File{}, ErrNotFound
	}
	return f, nil
}
func fixtureService(t *testing.T, options Options) (*Service, *memoryStore, string) {
	t.Helper()
	directory := t.TempDir()
	files, err := NewFileStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { files.Close() })
	store := &memoryStore{files: make(map[string]File)}
	return NewService(store, files, options), store, directory
}
func testPNG(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, 3, 2))
	picture.Set(0, 0, color.RGBA{R: 100, G: 80, B: 255, A: 255})
	if err := png.Encode(&out, picture); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func testJPEG(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := jpeg.Encode(&out, image.NewRGBA(image.Rect(0, 0, 3, 2)), nil); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func boxBytes(kind string, contents ...[]byte) []byte {
	payload := bytes.Join(contents, nil)
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out, uint32(len(out)))
	copy(out[4:8], kind)
	copy(out[8:], payload)
	return out
}
func testMP4() []byte {
	handler := make([]byte, 24)
	copy(handler[8:12], "vide")
	track := boxBytes("trak", boxBytes("tkhd", make([]byte, 24)), boxBytes("mdia", boxBytes("mdhd", make([]byte, 24)), boxBytes("hdlr", handler), boxBytes("minf", boxBytes("stbl", boxBytes("stsd", make([]byte, 16))))))
	return bytes.Join([][]byte{boxBytes("ftyp", []byte("isom\x00\x00\x00\x00isommp41")), boxBytes("moov", boxBytes("mvhd", make([]byte, 20)), track), boxBytes("mdat", []byte{0, 1, 2, 3})}, nil)
}
func ebmlBytes(id []byte, contents ...[]byte) []byte {
	payload := bytes.Join(contents, nil)
	if len(payload) >= 127 {
		panic("fixture too large")
	}
	return bytes.Join([][]byte{id, {byte(0x80 | len(payload))}, payload}, nil)
}
func testWebM() []byte {
	header := ebmlBytes([]byte{0x1a, 0x45, 0xdf, 0xa3}, ebmlBytes([]byte{0x42, 0x82}, []byte("webm")))
	tracks := ebmlBytes([]byte{0x16, 0x54, 0xae, 0x6b}, ebmlBytes([]byte{0xae}, ebmlBytes([]byte{0x83}, []byte{1}), ebmlBytes([]byte{0x86}, []byte("V_VP9"))))
	segment := ebmlBytes([]byte{0x18, 0x53, 0x80, 0x67}, ebmlBytes([]byte{0x15, 0x49, 0xa9, 0x66}, []byte{0}), tracks, ebmlBytes([]byte{0x1f, 0x43, 0xb6, 0x75}, []byte{0}))
	return append(header, segment...)
}
func pngWithDimensions(width, height uint32) []byte {
	payload := make([]byte, 13)
	binary.BigEndian.PutUint32(payload, width)
	binary.BigEndian.PutUint32(payload[4:], height)
	payload[8] = 8
	payload[9] = 2
	out := append([]byte{}, []byte("\x89PNG\r\n\x1a\n")...)
	chunk := make([]byte, 25)
	binary.BigEndian.PutUint32(chunk, 13)
	copy(chunk[4:], "IHDR")
	copy(chunk[8:], payload)
	binary.BigEndian.PutUint32(chunk[21:], crc32.ChecksumIEEE(chunk[4:21]))
	return append(out, chunk...)
}

func TestUploadVerifiedFormatsHashesAndPrivateStorage(t *testing.T) {
	for _, tc := range []struct {
		name, purpose, contentType string
		data                       []byte
	}{{"png", PurposeImage, "image/png", testPNG(t)}, {"jpeg", PurposeImage, "image/jpeg", testJPEG(t)}, {"mp4", PurposeVideo, "video/mp4", testMP4()}, {"webm", PurposeVideo, "video/webm", testWebM()}} {
		t.Run(tc.name, func(t *testing.T) {
			service, store, directory := fixtureService(t, Options{})
			file, err := service.Upload(context.Background(), staffUser(), UploadInput{Purpose: tc.purpose, Filename: "нұсқаулық." + tc.name, ContentType: tc.contentType, Reader: bytes.NewReader(tc.data)})
			if err != nil {
				t.Fatal(err)
			}
			if store.created != 1 || file.ByteCount != int64(len(tc.data)) || file.ContentType != tc.contentType || file.URL != "/api/v1/media/"+file.ID || !storageKeyPattern.MatchString(file.StorageKey) {
				t.Fatalf("invalid metadata: %+v", file)
			}
			digest := sha256.Sum256(tc.data)
			if !bytes.Equal(digest[:], file.SHA256) {
				t.Fatal("incorrect persisted SHA256")
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 1 || strings.HasPrefix(entries[0].Name(), ".upload-") {
				t.Fatalf("staging cleanup failed: %v %v", entries, err)
			}
			information, err := os.Stat(filepath.Join(directory, file.StorageKey))
			if err != nil || information.Mode().Perm() != 0600 {
				t.Fatalf("private mode %v %v", information, err)
			}
			_, download, err := service.Download(context.Background(), staffUser(), file.ID)
			if err != nil {
				t.Fatal(err)
			}
			defer download.Close()
			actual, err := io.ReadAll(download)
			if err != nil || !bytes.Equal(actual, tc.data) {
				t.Fatal("download differs")
			}
		})
	}
}
func TestUploadRejectsInvalidInputAndCleansStaging(t *testing.T) {
	pngData := testPNG(t)
	mp4 := testMP4()
	badBox := append([]byte{}, mp4...)
	binary.BigEndian.PutUint32(badBox[len(badBox)-12:], 999999)
	for _, tc := range []struct {
		name, filename, purpose, contentType string
		data                                 []byte
		expected                             error
	}{
		{"html", "screen.png", PurposeImage, "image/png", []byte("<html>attack</html>"), ErrInvalidFile},
		{"svg", "screen.svg", PurposeImage, "image/svg+xml", []byte(`<svg></svg>`), ErrInvalidFile},
		{"type mismatch", "screen.png", PurposeImage, "image/jpeg", pngData, ErrInvalidFile},
		{"truncated png", "screen.png", PurposeImage, "image/png", pngData[:len(pngData)-20], ErrInvalidFile},
		{"huge width", "screen.png", PurposeImage, "image/png", pngWithDimensions(10001, 1), ErrInvalidFile},
		{"huge pixels", "screen.png", PurposeImage, "image/png", pngWithDimensions(9000, 9000), ErrInvalidFile},
		{"mp4 signature only", "screen.mp4", PurposeVideo, "video/mp4", mp4[:20], ErrInvalidFile},
		{"mp4 invalid box size", "screen.mp4", PurposeVideo, "video/mp4", badBox, ErrInvalidFile},
		{"webm truncated", "screen.webm", PurposeVideo, "video/webm", testWebM()[:20], ErrInvalidFile},
		{"unsupported purpose", "screen.png", "UNSUPPORTED", "image/png", pngData, ErrInvalidInput},
		{"traversal filename", "../screen.png", PurposeImage, "image/png", pngData, ErrInvalidInput},
		{"backslash filename", `..\screen.png`, PurposeImage, "image/png", pngData, ErrInvalidInput},
		{"control filename", "screen\x00.png", PurposeImage, "image/png", pngData, ErrInvalidInput},
		{"empty", "screen.png", PurposeImage, "image/png", nil, ErrInvalidFile},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, store, directory := fixtureService(t, Options{})
			_, err := service.Upload(context.Background(), staffUser(), UploadInput{Purpose: tc.purpose, Filename: tc.filename, ContentType: tc.contentType, Reader: bytes.NewReader(tc.data)})
			if !errors.Is(err, tc.expected) {
				t.Fatalf("expected %v got %v", tc.expected, err)
			}
			entries, _ := os.ReadDir(directory)
			if store.created != 0 || len(entries) != 0 {
				t.Fatal("invalid file persisted")
			}
		})
	}
}

func TestEmployeeTicketScreenshotsKeepArticleUploadsRestricted(t *testing.T) {
	actor := auth.User{ID: testID, Role: auth.RoleEmployee, IsActive: true}
	for _, tc := range []struct {
		name, purpose string
		data          []byte
		want          error
	}{
		{"ticket PNG", PurposeTicket, testPNG(t), nil},
		{"ticket JPEG", PurposeTicket, testJPEG(t), nil},
		{"ticket rejects video", PurposeTicket, testMP4(), ErrInvalidFile},
		{"article image requires staff", PurposeImage, testPNG(t), ErrForbidden},
		{"article video requires staff", PurposeVideo, testMP4(), ErrForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, store, directory := fixtureService(t, Options{})
			file, err := service.Upload(context.Background(), actor, UploadInput{Purpose: tc.purpose, Filename: "screen.png", Reader: bytes.NewReader(tc.data)})
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
			entries, readErr := os.ReadDir(directory)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if tc.want == nil {
				if file.Purpose != PurposeTicket || store.created != 1 || len(entries) != 1 {
					t.Fatal("screenshot not persisted privately")
				}
			} else if store.created != 0 || len(entries) != 0 {
				t.Fatal("rejected upload persisted bytes or metadata")
			}
		})
	}
}
func TestUploadOversizeDatabaseFailureAndCancellationCleanup(t *testing.T) {
	t.Run("limit", func(t *testing.T) {
		service, store, directory := fixtureService(t, Options{MaxImageBytes: 16})
		_, err := service.Upload(context.Background(), staffUser(), UploadInput{Purpose: PurposeImage, Filename: "image.png", Reader: bytes.NewReader(testPNG(t))})
		if !errors.Is(err, ErrTooLarge) {
			t.Fatal(err)
		}
		entries, _ := os.ReadDir(directory)
		if store.created != 0 || len(entries) != 0 {
			t.Fatal("overlimit data persisted")
		}
	})
	t.Run("database failure", func(t *testing.T) {
		service, store, directory := fixtureService(t, Options{})
		store.err = errors.New("synthetic metadata failure")
		_, err := service.Upload(context.Background(), staffUser(), UploadInput{Purpose: PurposeImage, Filename: "image.png", Reader: bytes.NewReader(testPNG(t))})
		if !errors.Is(err, store.err) {
			t.Fatal(err)
		}
		entries, _ := os.ReadDir(directory)
		if len(entries) != 0 {
			t.Fatal("final file survived failed metadata transaction")
		}
	})
	t.Run("uncertain commit preserves private file", func(t *testing.T) {
		service, store, directory := fixtureService(t, Options{})
		store.err = errors.Join(ErrCommitUncertain, errors.New("synthetic lost COMMIT acknowledgement"))
		_, err := service.Upload(context.Background(), staffUser(), UploadInput{Purpose: PurposeImage, Filename: "image.png", Reader: bytes.NewReader(testPNG(t))})
		if !errors.Is(err, ErrCommitUncertain) {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(directory)
		if err != nil || len(entries) != 1 || strings.HasPrefix(entries[0].Name(), ".upload-") {
			t.Fatal("potentially committed file must remain available for reconciliation")
		}
	})
	t.Run("cancellation", func(t *testing.T) {
		service, store, directory := fixtureService(t, Options{})
		ctx, cancel := context.WithCancel(context.Background())
		reader := cancelReader{cancel, bytes.NewReader(testPNG(t))}
		_, err := service.Upload(ctx, staffUser(), UploadInput{Purpose: PurposeImage, Filename: "image.png", Reader: reader})
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		entries, _ := os.ReadDir(directory)
		if store.created != 0 || len(entries) != 0 {
			t.Fatal("canceled data persisted")
		}
	})
	for _, actor := range []auth.User{{ID: testID, IsActive: true, Role: auth.RoleEmployee}, {ID: testID, IsActive: false, Role: auth.RoleAdmin}} {
		service, store, _ := fixtureService(t, Options{})
		if _, err := service.Upload(context.Background(), actor, UploadInput{}); !errors.Is(err, ErrForbidden) || store.created != 0 {
			t.Fatal("unauthorized upload accepted")
		}
	}
}

type cancelReader struct {
	cancel context.CancelFunc
	reader io.Reader
}

func (r cancelReader) Read(b []byte) (int, error) {
	n, err := r.reader.Read(b)
	r.cancel()
	return n, err
}
func TestFileStoreRejectsTraversalSymlinksAndChangedFiles(t *testing.T) {
	service, store, directory := fixtureService(t, Options{})
	outside := filepath.Join(t.TempDir(), "private.png")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	key := strings.Repeat("a", 32) + ".png"
	if err := os.Symlink(outside, filepath.Join(directory, key)); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../private.png", outside, key, key + "/", ".upload-" + strings.Repeat("a", 32)} {
		if f, err := service.files.Open(path); err == nil {
			f.Close()
			t.Fatalf("unsafe path allowed: %q", path)
		}
	}
	if err := os.Remove(filepath.Join(directory, key)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, key), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	alias := strings.Repeat("b", 32) + ".png"
	if err := os.Symlink(key, filepath.Join(directory, alias)); err != nil {
		t.Fatal(err)
	}
	if f, err := service.files.Open(alias); err == nil {
		f.Close()
		t.Fatal("in-root symlink allowed")
	}
	store.files[testID] = File{ID: testID, StorageKey: key, ByteCount: 5}
	if _, f, err := service.Download(context.Background(), staffUser(), testID); !errors.Is(err, ErrNotFound) {
		if f != nil {
			f.Close()
		}
		t.Fatal("changed size allowed")
	}
}
