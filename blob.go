package imagor

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cshum/imagor/fanoutreader"
	"github.com/cshum/imagor/seekstream"
)

// BlobType blob content type
type BlobType int

const maxMemorySize = int64(100 << 20) // 100MB

// BlobType enum
const (
	BlobTypeUnknown BlobType = iota
	BlobTypeEmpty
	BlobTypeMemory
	BlobTypeJSON
	BlobTypeJPEG
	BlobTypePNG
)

// Blob imagor data blob abstraction
type Blob struct {
	newReader     func() (r io.ReadCloser, size int64, err error)
	newReadSeeker func() (rs io.ReadSeekCloser, size int64, err error)
	fanout        bool
	once          sync.Once
	sniffBuf      []byte
	err           error
	size          int64
	blobType      BlobType
	filepath      string
	contentType   string
	memory        *memory

	Stat *Stat
}

// Stat Blob stat attributes
type Stat struct {
	ModifiedTime time.Time
	ETag         string
	Size         int64
}

// NewBlob creates imagor Blob from io.ReadCloser and size
func NewBlob(newReader func() (reader io.ReadCloser, size int64, err error)) *Blob {
	return &Blob{
		fanout:    true,
		newReader: newReader,
	}
}

// NewBlobFromFile creates imagor Blob from file path and optional file info checks
func NewBlobFromFile(filepath string, checks ...func(os.FileInfo) error) *Blob {
	stat, err := os.Stat(filepath)
	if os.IsNotExist(err) {
		err = ErrNotFound
	}
	if err == nil {
		for _, check := range checks {
			if err = check(stat); err != nil {
				break
			}
		}
	}
	blob := &Blob{
		err:      err,
		filepath: filepath,
		fanout:   true,
		newReader: func() (io.ReadCloser, int64, error) {
			if err != nil {
				return nil, 0, err
			}
			reader, err := os.Open(filepath)
			return reader, stat.Size(), err
		},
	}
	if err == nil && stat != nil {
		size := stat.Size()
		modTime := stat.ModTime()
		blob.Stat = &Stat{
			Size:         size,
			ModifiedTime: modTime,
		}
	}
	return blob
}

// NewBlobFromJsonMarshal creates imagor Blob from json marshal of any object
func NewBlobFromJsonMarshal(v any) *Blob {
	buf, err := json.Marshal(v)
	size := int64(len(buf))
	return &Blob{
		err:      err,
		blobType: BlobTypeJSON,
		fanout:   false,
		newReader: func() (io.ReadCloser, int64, error) {
			rs := bytes.NewReader(buf)
			return &readSeekNopCloser{ReadSeeker: rs}, size, err
		},
	}
}

// NewBlobFromBytes creates imagor Blob from []byte buffer
func NewBlobFromBytes(buf []byte) *Blob {
	size := int64(len(buf))
	return &Blob{
		fanout: false,
		newReader: func() (io.ReadCloser, int64, error) {
			rs := bytes.NewReader(buf)
			return &readSeekNopCloser{ReadSeeker: rs}, size, nil
		},
	}
}

// NewBlobFromMemory creates imagor Blob from raw RGB/RGBA buffer
func NewBlobFromMemory(buf []byte, width, height, bands int) *Blob {
	return &Blob{memory: &memory{
		data:   buf,
		width:  width,
		height: height,
		bands:  bands,
	}}
}

// NewEmptyBlob creates empty imagor Blob
func NewEmptyBlob() *Blob {
	return &Blob{}
}

var jpegHeader = []byte("\xFF\xD8\xFF")
var pngHeader = []byte("\x89\x50\x4E\x47")
var jsonPrefix = []byte(`{"`)

type readSeekNopCloser struct {
	io.ReadSeeker
}

func (readSeekNopCloser) Close() error { return nil }

// hybridReadSeeker uses io.ReadCloser and switch to io.ReadSeekCloser only when seeked
type hybridReadSeeker struct {
	reader        io.ReadCloser
	seeker        io.ReadSeekCloser
	newReadSeeker func() (io.ReadSeekCloser, int64, error)
}

// Read implements the io.Reader interface.
func (h *hybridReadSeeker) Read(p []byte) (n int, err error) {
	return h.reader.Read(p)
}

// Seek implements the io.Seeker interface.
func (h *hybridReadSeeker) Seek(offset int64, whence int) (_ int64, err error) {
	if h.seeker != nil {
		return h.seeker.Seek(offset, whence)
	}
	if h.seeker, _, err = h.newReadSeeker(); err != nil {
		return
	}
	_ = h.reader.Close()
	h.reader = h.seeker
	return h.seeker.Seek(offset, whence)
}

// Close implements the io.Closer interface.
func (h *hybridReadSeeker) Close() (err error) {
	return h.reader.Close()
}

type memory struct {
	data   []byte
	width  int
	height int
	bands  int
}

func newEmptyReader() (io.ReadCloser, int64, error) {
	return &readSeekNopCloser{bytes.NewReader(nil)}, 0, nil
}

func (b *Blob) init() {
	b.once.Do(b.doInit)
}

func (b *Blob) doInit() {
	if b.err != nil {
		return
	}
	if b.newReader == nil {
		if b.memory != nil {
			b.blobType = BlobTypeMemory
		} else {
			b.blobType = BlobTypeEmpty
		}
		b.newReader = newEmptyReader
		return
	}
	reader, size, err := b.newReader()
	if err != nil {
		b.err = err
	}
	if reader == nil {
		return
	}
	b.size = size
	if _, ok := reader.(io.ReadSeekCloser); ok {
		// construct seeker factory if source supports seek
		newReader := b.newReader
		b.newReadSeeker = func() (io.ReadSeekCloser, int64, error) {
			r, size, err := newReader()
			return r.(io.ReadSeekCloser), size, err
		}
	}
	if b.fanout && size > 0 && size < maxMemorySize && err == nil {
		// use fan-out reader if buf size known and within memory size
		// otherwise create new readers
		fanout := fanoutreader.New(reader, int(size))
		b.newReader = func() (io.ReadCloser, int64, error) {
			return fanout.NewReader(), size, nil
		}
		reader = fanout.NewReader()
		if b.newReadSeeker != nil {
			newReadSeeker := b.newReadSeeker
			b.newReadSeeker = func() (rs io.ReadSeekCloser, _ int64, err error) {
				return &hybridReadSeeker{
					reader:        fanout.NewReader(),
					newReadSeeker: newReadSeeker,
				}, size, nil
			}
		}
	} else {
		b.fanout = false
	}
	// sniff first 512 bytes for type sniffing
	b.sniffBuf = make([]byte, 512)
	n, err := io.ReadAtLeast(reader, b.sniffBuf, 512)
	_ = reader.Close()
	if n < 512 {
		b.sniffBuf = b.sniffBuf[:n]
	}
	if len(b.sniffBuf) == 0 {
		b.blobType = BlobTypeEmpty
	}
	if err != nil &&
		err != io.ErrUnexpectedEOF &&
		err != io.EOF {
		if b.err == nil {
			b.err = err
		}
		return
	}
	if b.blobType != BlobTypeEmpty && b.blobType != BlobTypeJSON &&
		len(b.sniffBuf) > 24 {
		if bytes.Equal(b.sniffBuf[:3], jpegHeader) {
			b.blobType = BlobTypeJPEG
		} else if bytes.Equal(b.sniffBuf[:4], pngHeader) {
			b.blobType = BlobTypePNG
		}
	}
	if b.contentType == "" {
		switch b.blobType {
		case BlobTypeJSON:
			b.contentType = "application/json"
		case BlobTypeJPEG:
			b.contentType = "image/jpeg"
		case BlobTypePNG:
			b.contentType = "image/png"
		default:
			b.contentType = http.DetectContentType(b.sniffBuf)
		}
	}
	if b.blobType == BlobTypeUnknown &&
		strings.HasPrefix(b.contentType, "text/plain") {
		if bytes.Equal(b.sniffBuf[:2], jsonPrefix) {
			b.blobType = BlobTypeJSON
			b.contentType = "application/json"
		}
	}
}

// IsEmpty check if blob is empty
func (b *Blob) IsEmpty() bool {
	b.init()
	return b.blobType == BlobTypeEmpty
}

// BlobType returns BlobType
func (b *Blob) BlobType() BlobType {
	b.init()
	return b.blobType
}

// Sniff returns first 512 bytes of blob data for type sniffing
func (b *Blob) Sniff() []byte {
	b.init()
	return b.sniffBuf
}

// Size returns Blob size if known
func (b *Blob) Size() int64 {
	b.init()
	return b.size
}

// FilePath returns Blob file path if blob is created from file
func (b *Blob) FilePath() string {
	return b.filepath
}

// Memory returns memory data if Blob is created from memory
func (b *Blob) Memory() (data []byte, width, height, bands int, ok bool) {
	if m := b.memory; m != nil {
		data = m.data
		width = m.width
		height = m.height
		bands = m.bands
		ok = true
	}
	return
}

// SetContentType set Blob content type. which overrides default sniffing if this is set
func (b *Blob) SetContentType(contentType string) {
	b.contentType = contentType
}

// ContentType returns content type
func (b *Blob) ContentType() string {
	b.init()
	return b.contentType
}

// NewReader creates new io.ReadCloser and returns size if known
func (b *Blob) NewReader() (reader io.ReadCloser, size int64, err error) {
	b.init()
	return b.newReader()
}

// NewReadSeeker create read seeker if reader supports seek,
// or attempts to simulate seek using memory or temp file buffer
func (b *Blob) NewReadSeeker() (io.ReadSeekCloser, int64, error) {
	b.init()
	if b.newReadSeeker != nil {
		return b.newReadSeeker()
	}
	// if source not seekable, simulate seek with seek stream
	reader, size, err := b.NewReader()
	if err != nil {
		return nil, size, err
	}
	var buffer seekstream.Buffer
	if size > 0 && size < maxMemorySize {
		// in memory buffer if size is known and less then 100mb
		buffer = seekstream.NewMemoryBuffer(size)
	} else {
		// otherwise temp file buffer
		buffer, err = seekstream.NewTempFileBuffer("", "imagor-")
		if err != nil {
			return nil, size, err
		}
	}
	return seekstream.New(reader, buffer), size, err
}

// ReadAll real all bytes from Blob
func (b *Blob) ReadAll() ([]byte, error) {
	b.init()
	if b.blobType == BlobTypeEmpty {
		return nil, b.err
	}
	reader, size, err := b.NewReader()
	if reader != nil {
		defer func() {
			_ = reader.Close()
		}()
		if size > 0 {
			buf := make([]byte, size)
			s := int(size)
			n, err2 := io.ReadAtLeast(reader, buf, s)
			if n < s {
				buf = buf[:n]
			}
			if err != nil {
				return buf, err
			}
			return buf, err2
		}
		buf, err2 := io.ReadAll(reader)
		if err != nil {
			return buf, err
		}
		return buf, err2
	}
	return nil, err
}

// Err returns Blob error
func (b *Blob) Err() error {
	b.init()
	return b.err
}

func isBlobEmpty(blob *Blob) bool {
	return blob == nil || blob.IsEmpty()
}

func checkBlob(blob *Blob, err error) (*Blob, error) {
	if blob != nil && err == nil {
		err = blob.Err()
	}
	return blob, err
}

func getExtension(typ BlobType) (ext string) {
	switch typ {
	case BlobTypeJPEG:
		ext = ".jpg"
	case BlobTypePNG:
		ext = ".png"
	case BlobTypeJSON:
		ext = ".json"
	}
	return
}
