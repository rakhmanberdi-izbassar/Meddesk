package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"strings"
)

func validateFile(ctx context.Context, f *os.File, size int64, purpose string) (string, string, error) {
	header := make([]byte, 512)
	n, err := f.ReadAt(header, 0)
	if err != nil && err != io.EOF {
		return "", "", err
	}
	header = header[:n]
	detected := http.DetectContentType(header)
	if purpose == PurposeImage || purpose == PurposeTicket {
		extension := ""
		switch detected {
		case "image/png":
			extension = "png"
		case "image/jpeg":
			extension = "jpg"
		default:
			return "", "", ErrInvalidFile
		}
		if _, err = f.Seek(0, io.SeekStart); err != nil {
			return "", "", err
		}
		config, format, err := image.DecodeConfig(contextReader{ctx, f})
		if err != nil || format != extension && !(format == "jpeg" && extension == "jpg") {
			return "", "", ErrInvalidFile
		}
		if config.Width < 1 || config.Height < 1 || config.Width > 10000 || config.Height > 10000 || int64(config.Width)*int64(config.Height) > 40_000_000 {
			return "", "", ErrInvalidFile
		}
		if _, err = f.Seek(0, io.SeekStart); err != nil {
			return "", "", err
		}
		decoded, actual, err := image.Decode(contextReader{ctx, f})
		if err != nil || actual != format || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height {
			if ctx.Err() != nil {
				return "", "", ctx.Err()
			}
			return "", "", ErrInvalidFile
		}
		return detected, extension, nil
	}
	if purpose != PurposeVideo {
		return "", "", ErrInvalidInput
	}
	switch detected {
	case "video/mp4":
		if err := validateMP4(ctx, f, size); err != nil {
			return "", "", err
		}
		return detected, "mp4", nil
	case "video/webm":
		if err := validateWebM(ctx, f, size); err != nil {
			return "", "", err
		}
		return detected, "webm", nil
	default:
		return "", "", ErrInvalidFile
	}
}

type box struct {
	name       string
	start, end int64
}

func mp4Boxes(ctx context.Context, r io.ReaderAt, start, end int64) ([]box, error) {
	boxes := make([]box, 0, 8)
	for offset := start; offset < end; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(boxes) >= 10000 || end-offset < 8 {
			return nil, ErrInvalidFile
		}
		var header [16]byte
		if _, err := r.ReadAt(header[:8], offset); err != nil {
			return nil, ErrInvalidFile
		}
		length := int64(binary.BigEndian.Uint32(header[:4]))
		headerSize := int64(8)
		name := string(header[4:8])
		if length == 1 {
			if end-offset < 16 {
				return nil, ErrInvalidFile
			}
			if _, err := r.ReadAt(header[8:], offset+8); err != nil {
				return nil, ErrInvalidFile
			}
			unsigned := binary.BigEndian.Uint64(header[8:])
			if unsigned > uint64(end-offset) {
				return nil, ErrInvalidFile
			}
			length = int64(unsigned)
			headerSize = 16
		}
		if length == 0 {
			if name != "mdat" {
				return nil, ErrInvalidFile
			}
			length = end - offset
		}
		if length < headerSize || length > end-offset {
			return nil, ErrInvalidFile
		}
		boxes = append(boxes, box{name, offset + headerSize, offset + length})
		offset += length
	}
	return boxes, nil
}
func validateMP4(ctx context.Context, f *os.File, size int64) error {
	boxes, err := mp4Boxes(ctx, f, 0, size)
	if err != nil {
		return err
	}
	if len(boxes) < 3 || boxes[0].name != "ftyp" {
		return ErrInvalidFile
	}
	brand := boxes[0]
	if brand.end-brand.start < 8 || brand.end-brand.start > 4096 || (brand.end-brand.start)%4 != 0 {
		return ErrInvalidFile
	}
	movie, data := false, false
	for _, b := range boxes {
		switch b.name {
		case "moov":
			if movie {
				return ErrInvalidFile
			}
			movie = true
			children, err := mp4Boxes(ctx, f, b.start, b.end)
			if err != nil {
				return err
			}
			mvhd, track := false, false
			for _, child := range children {
				switch child.name {
				case "mvhd":
					mvhd = child.end-child.start >= 20
				case "trak":
					valid, err := mp4VideoTrack(ctx, f, child)
					if err != nil {
						return err
					}
					track = track || valid
				}
			}
			if !mvhd || !track {
				return ErrInvalidFile
			}
		case "mdat":
			data = data || b.end > b.start
		case "ftyp":
			if b.start != brand.start {
				return ErrInvalidFile
			}
		}
	}
	if !movie || !data {
		return ErrInvalidFile
	}
	return nil
}
func mp4VideoTrack(ctx context.Context, r io.ReaderAt, track box) (bool, error) {
	children, err := mp4Boxes(ctx, r, track.start, track.end)
	if err != nil {
		return false, err
	}
	tkhd, video := false, false
	for _, b := range children {
		switch b.name {
		case "tkhd":
			tkhd = b.end-b.start >= 24
		case "mdia":
			fields, err := mp4Boxes(ctx, r, b.start, b.end)
			if err != nil {
				return false, err
			}
			mdhd, handler, minf := false, false, false
			for _, field := range fields {
				switch field.name {
				case "mdhd":
					mdhd = field.end-field.start >= 24
				case "hdlr":
					if field.end-field.start < 24 {
						return false, ErrInvalidFile
					}
					var h [12]byte
					if _, err := r.ReadAt(h[:], field.start); err != nil {
						return false, ErrInvalidFile
					}
					handler = string(h[8:12]) == "vide"
				case "minf":
					contents, err := mp4Boxes(ctx, r, field.start, field.end)
					if err != nil {
						return false, err
					}
					for _, c := range contents {
						if c.name == "stbl" {
							samples, err := mp4Boxes(ctx, r, c.start, c.end)
							if err != nil {
								return false, err
							}
							for _, s := range samples {
								if s.name == "stsd" && s.end-s.start >= 16 {
									minf = true
								}
							}
						}
					}
				}
			}
			video = video || (mdhd && handler && minf)
		}
	}
	return tkhd && video, nil
}

// WebM uses EBML variable-length IDs and lengths. Validation bounds every
// element and verifies the WebM document type, a video codec track, and clusters.
// It validates the container; codec decoding and transcoding are not performed.
type element struct {
	id         uint64
	start, end int64
	unknown    bool
}

func ebmlVInt(r io.ReaderAt, offset, end int64, id bool) (uint64, int, bool, error) {
	if offset >= end {
		return 0, 0, false, ErrInvalidFile
	}
	var b [8]byte
	if _, err := r.ReadAt(b[:1], offset); err != nil {
		return 0, 0, false, ErrInvalidFile
	}
	width := 1
	mask := byte(0x80)
	for mask != 0 && b[0]&mask == 0 {
		width++
		mask >>= 1
	}
	if mask == 0 || width > 8 || (id && width > 4) || end-offset < int64(width) {
		return 0, 0, false, ErrInvalidFile
	}
	if _, err := r.ReadAt(b[:width], offset); err != nil {
		return 0, 0, false, ErrInvalidFile
	}
	value := uint64(b[0])
	if !id {
		value &= uint64(mask - 1)
	}
	for _, part := range b[1:width] {
		value = value<<8 | uint64(part)
	}
	unknown := !id && value == (uint64(1)<<(7*width))-1
	return value, width, unknown, nil
}
func ebmlElement(r io.ReaderAt, offset, end int64) (element, error) {
	id, n, _, err := ebmlVInt(r, offset, end, true)
	if err != nil {
		return element{}, err
	}
	length, m, unknown, err := ebmlVInt(r, offset+int64(n), end, false)
	if err != nil {
		return element{}, err
	}
	start := offset + int64(n+m)
	if unknown {
		return element{id, start, end, true}, nil
	}
	if length > uint64(end-start) {
		return element{}, ErrInvalidFile
	}
	return element{id, start, start + int64(length), false}, nil
}
func ebmlChildren(ctx context.Context, r io.ReaderAt, start, end int64) ([]element, error) {
	children := make([]element, 0, 8)
	for offset := start; offset < end; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(children) >= 10000 {
			return nil, ErrInvalidFile
		}
		e, err := ebmlElement(r, offset, end)
		if err != nil {
			return nil, err
		}
		children = append(children, e)
		offset = e.end
	}
	return children, nil
}
func validateWebM(ctx context.Context, f *os.File, size int64) error {
	header, err := ebmlElement(f, 0, size)
	if err != nil || header.id != 0x1a45dfa3 || header.unknown || header.end-header.start > 4096 {
		return ErrInvalidFile
	}
	fields, err := ebmlChildren(ctx, f, header.start, header.end)
	if err != nil {
		return err
	}
	doctype := false
	for _, field := range fields {
		if field.id == 0x4282 {
			if field.unknown || field.end-field.start != 4 {
				return ErrInvalidFile
			}
			var text [4]byte
			if _, err := f.ReadAt(text[:], field.start); err != nil {
				return ErrInvalidFile
			}
			doctype = bytes.Equal(text[:], []byte("webm"))
		}
	}
	if !doctype {
		return ErrInvalidFile
	}
	segment, err := ebmlElement(f, header.end, size)
	if err != nil || segment.id != 0x18538067 || segment.end != size {
		return ErrInvalidFile
	}
	children, err := ebmlChildren(ctx, f, segment.start, segment.end)
	if err != nil {
		return err
	}
	info, video, cluster := false, false, false
	for _, child := range children {
		switch child.id {
		case 0x1549a966:
			info = child.end > child.start && !child.unknown
		case 0x1654ae6b:
			if child.unknown || child.end-child.start > 1<<20 {
				return ErrInvalidFile
			}
			tracks, err := ebmlChildren(ctx, f, child.start, child.end)
			if err != nil {
				return err
			}
			for _, track := range tracks {
				if track.id != 0xae {
					continue
				}
				if track.unknown {
					return ErrInvalidFile
				}
				fields, err := ebmlChildren(ctx, f, track.start, track.end)
				if err != nil {
					return err
				}
				videoType, codec := false, false
				for _, field := range fields {
					switch field.id {
					case 0x83:
						if field.end-field.start != 1 || field.unknown {
							return ErrInvalidFile
						}
						var kind [1]byte
						if _, err := f.ReadAt(kind[:], field.start); err != nil {
							return ErrInvalidFile
						}
						videoType = kind[0] == 1
					case 0x86:
						if field.unknown || field.end-field.start > 32 {
							return ErrInvalidFile
						}
						value := make([]byte, field.end-field.start)
						if _, err := f.ReadAt(value, field.start); err != nil {
							return ErrInvalidFile
						}
						codec = strings.EqualFold(string(value), "V_VP8") || strings.EqualFold(string(value), "V_VP9") || strings.EqualFold(string(value), "V_AV1")
					}
				}
				video = video || (videoType && codec)
			}
		case 0x1f43b675:
			cluster = cluster || child.end > child.start
		default:
			if child.unknown {
				return ErrInvalidFile
			}
		}
	}
	if !info || !video || !cluster {
		return ErrInvalidFile
	}
	return nil
}
