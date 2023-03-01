package sketch

// #cgo CFLAGS: -I.
// #cgo LDFLAGS: -lrasterizer
// #include "PRRasterizerC.h"
import "C"

import (
	"context"
	"fmt"
	"strings"
	"unsafe"

	"github.com/cshum/imagor"
	"github.com/cshum/imagor/imagorpath"
	"go.uber.org/zap"
)

// Implements the interface imagor.Processor
type Processor struct {
	Disabled bool
	Debug    bool
	Logger   *zap.Logger
}

func NewProcessor(options ...Option) *Processor {
	p := &Processor{
		Disabled: false,
		Debug:    false,
		Logger:   zap.NewNop(),
	}
	for _, opt := range options {
		opt(p)
	}
	return p
}

// Startup implements imagor.Processor interface
func (p *Processor) Startup(_ context.Context) error {
	if p.Debug {
		p.Logger.Debug("sketch", zap.String("log", "Processor startup"))
	}
	return nil
}

// Shutdown implements imagor.Processor interface
func (p *Processor) Shutdown(_ context.Context) error {
	if p.Debug {
		p.Logger.Debug("sketch", zap.String("log", "Processor shutdown"))
	}
	return nil
}

// Process implements imagor.Processor interface
func (p *Processor) Process(ctx context.Context, blob *imagor.Blob, params imagorpath.Params, load imagor.LoadFunc) (*imagor.Blob, error) {
	if p.Debug {
		p.Logger.Debug("sketch", zap.String("log", "Processor process"))
	}

	//TODO: Detect if we should process the blob not by checking the file extension
	if !strings.HasSuffix(blob.FilePath(), ".sketchpresentation") {
		p.Logger.Info("sketch", zap.String("log", "Not a sketch presentation file, forwarding request"))
		return nil, imagor.ErrForward{Params: params}
	}

	p.Logger.Info("sketch", zap.String("log", fmt.Sprint("Reading presentation file: ", blob.FilePath())))
	data, err := blob.ReadAll()
	if err != nil {
		return nil, imagor.WrapError(err)
	}
	dataPtr := (*C.char)(unsafe.Pointer(&data[0]))
	size := C.ulong(len(data))
	c := C.PRRasterizerNew(dataPtr, size)

	p.Logger.Info("sketch", zap.String("log", "Rasterazing presentation file"))
	r := C.PRRasterizerExportPNG(c, 2, 100)
	rData := unsafe.Pointer(r.buffer)
	rSize := C.int(r.size)
	rBuffer := C.GoBytes(rData, rSize)

	p.Logger.Info("sketch", zap.String("log", "Converting presentation file raster to blob"))
	b := imagor.NewBlobFromBytes(rBuffer)
	b.SetContentType("image/png")

	C.PRRasterizerResultFree(r)
	C.PRRasterizerFree(c)

	//We return ErrForward to indicate that we want to forward the request to the next processor
	return b, imagor.ErrForward{Params: params}
}

type Option func(*Processor)

func WithDisableFormat(disabled bool) Option {
	return func(p *Processor) {
		p.Disabled = disabled
	}
}

func WithLogger(logger *zap.Logger) Option {
	return func(p *Processor) {
		if logger != nil {
			p.Logger = logger
		}
	}
}

func WithDebug(debug bool) Option {
	return func(p *Processor) {
		p.Debug = debug
	}
}
