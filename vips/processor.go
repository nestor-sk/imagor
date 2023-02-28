package vips

import (
	"context"
	"math"
	"runtime"
	"strings"
	"sync"

	"github.com/cshum/imagor"
	"go.uber.org/zap"
)

// FilterFunc filter handler function
type FilterFunc func(ctx context.Context, img *Image, load imagor.LoadFunc, args ...string) (err error)

// FilterMap filter handler map
type FilterMap map[string]FilterFunc

var processorLock sync.RWMutex
var processorCount int

// Processor implements imagor.Processor interface
type Processor struct {
	Filters            FilterMap
	DisableBlur        bool
	DisableFilters     []string
	MaxFilterOps       int
	Logger             *zap.Logger
	Concurrency        int
	MaxCacheFiles      int
	MaxCacheMem        int
	MaxCacheSize       int
	MaxWidth           int
	MaxHeight          int
	MaxResolution      int
	MaxAnimationFrames int
	MozJPEG            bool
	Debug              bool

	disableFilters map[string]bool
}

// NewProcessor create Processor
func NewProcessor(options ...Option) *Processor {
	v := &Processor{
		MaxWidth:           9999,
		MaxHeight:          9999,
		MaxResolution:      81000000,
		Concurrency:        1,
		MaxFilterOps:       -1,
		MaxAnimationFrames: -1,
		Logger:             zap.NewNop(),
		disableFilters:     map[string]bool{},
	}
	v.Filters = FilterMap{
		"watermark":        v.watermark,
		"round_corner":     roundCorner,
		"rotate":           rotate,
		"label":            label,
		"grayscale":        grayscale,
		"brightness":       brightness,
		"background_color": backgroundColor,
		"contrast":         contrast,
		"modulate":         modulate,
		"hue":              hue,
		"saturation":       saturation,
		"rgb":              rgb,
		"blur":             blur,
		"sharpen":          sharpen,
		"strip_icc":        stripIcc,
		"strip_exif":       stripExif,
		"trim":             trim,
		"set_frames":       setFrames,
		"padding":          v.padding,
		"proportion":       proportion,
	}
	for _, option := range options {
		option(v)
	}
	if v.DisableBlur {
		v.DisableFilters = append(v.DisableFilters, "blur", "sharpen")
	}
	for _, name := range v.DisableFilters {
		v.disableFilters[name] = true
	}
	if v.Concurrency == -1 {
		v.Concurrency = runtime.NumCPU()
	}
	return v
}

// Startup implements imagor.Processor interface
func (v *Processor) Startup(_ context.Context) error {
	processorLock.Lock()
	defer processorLock.Unlock()
	processorCount++
	if processorCount > 1 {
		return nil
	}
	if v.Debug {
		SetLogging(func(domain string, level LogLevel, msg string) {
			switch level {
			case LogLevelDebug:
				v.Logger.Debug(domain, zap.String("log", msg))
			case LogLevelMessage, LogLevelInfo:
				v.Logger.Info(domain, zap.String("log", msg))
			case LogLevelWarning, LogLevelCritical, LogLevelError:
				v.Logger.Warn(domain, zap.String("log", msg))
			}
		}, LogLevelDebug)
	} else {
		SetLogging(func(domain string, level LogLevel, msg string) {
			v.Logger.Warn(domain, zap.String("log", msg))
		}, LogLevelError)
	}
	Startup(&Config{
		MaxCacheFiles:    v.MaxCacheFiles,
		MaxCacheMem:      v.MaxCacheMem,
		MaxCacheSize:     v.MaxCacheSize,
		ConcurrencyLevel: v.Concurrency,
	})
	return nil
}

// Shutdown implements imagor.Processor interface
func (v *Processor) Shutdown(_ context.Context) error {
	processorLock.Lock()
	defer processorLock.Unlock()
	if processorCount <= 0 {
		return nil
	}
	processorCount--
	if processorCount == 0 {
		Shutdown()
	}
	return nil
}

func newImageFromBlob(
	ctx context.Context, blob *imagor.Blob, params *ImportParams,
) (*Image, error) {
	if blob == nil || blob.IsEmpty() {
		return nil, imagor.ErrNotFound
	}
	if blob.BlobType() == imagor.BlobTypeMemory {
		buf, width, height, bands, _ := blob.Memory()
		return LoadImageFromMemory(buf, width, height, bands)
	}
	reader, _, err := blob.NewReader()
	if err != nil {
		return nil, err
	}
	src := NewSource(reader)
	contextDefer(ctx, src.Close)
	img, err := src.LoadImage(params)
	return img, err
}

func newThumbnailFromBlob(
	ctx context.Context, blob *imagor.Blob,
	width, height int, crop Interesting, size Size, params *ImportParams,
) (*Image, error) {
	if blob == nil || blob.IsEmpty() {
		return nil, imagor.ErrNotFound
	}
	reader, _, err := blob.NewReader()
	if err != nil {
		return nil, err
	}
	src := NewSource(reader)
	contextDefer(ctx, src.Close)
	return src.LoadThumbnail(width, height, crop, size, params)
}

// NewThumbnail creates new thumbnail with resize and crop from imagor.Blob
func (v *Processor) NewThumbnail(
	ctx context.Context, blob *imagor.Blob, width, height int, crop Interesting, size Size, n int,
) (*Image, error) {
	var params = NewImportParams()
	var err error
	var img *Image
	params.FailOnError.Set(false)
	switch blob.BlobType() {
	case imagor.BlobTypeJPEG:
		// only allow real thumbnail for jpeg gif webp
		img, err = newThumbnailFromBlob(ctx, blob, width, height, crop, size, params)
	default:
		img, err = v.newThumbnailFallback(ctx, blob, width, height, crop, size, params)
	}
	return v.CheckResolution(img, WrapErr(err))
}

func (v *Processor) newThumbnailFallback(
	ctx context.Context, blob *imagor.Blob, width, height int, crop Interesting, size Size, params *ImportParams,
) (img *Image, err error) {
	if img, err = v.CheckResolution(newImageFromBlob(ctx, blob, params)); err != nil {
		return
	}
	if err = img.ThumbnailWithSize(width, height, crop, size); err != nil {
		img.Close()
		return
	}
	return img, WrapErr(err)
}

// NewImage creates new Image from imagor.Blob
func (v *Processor) NewImage(ctx context.Context, blob *imagor.Blob, n int) (*Image, error) {
	var params = NewImportParams()
	params.FailOnError.Set(false)
	img, err := v.CheckResolution(newImageFromBlob(ctx, blob, params))
	if err != nil {
		return nil, WrapErr(err)
	}
	return img, nil
}

// Thumbnail handles thumbnail operation
func (v *Processor) Thumbnail(
	img *Image, width, height int, crop Interesting, size Size,
) error {
	if crop == InterestingNone || size == SizeForce || img.Height() == img.PageHeight() {
		return img.ThumbnailWithSize(width, height, crop, size)
	}
	return v.animatedThumbnailWithCrop(img, width, height, crop, size)
}

// FocalThumbnail handles thumbnail with custom focal point
func (v *Processor) FocalThumbnail(img *Image, w, h int, fx, fy float64) (err error) {
	if float64(w)/float64(h) > float64(img.Width())/float64(img.PageHeight()) {
		if err = img.Thumbnail(w, v.MaxHeight, InterestingNone); err != nil {
			return
		}
	} else {
		if err = img.Thumbnail(v.MaxWidth, h, InterestingNone); err != nil {
			return
		}
	}
	var top, left float64
	left = float64(img.Width())*fx - float64(w)/2
	top = float64(img.PageHeight())*fy - float64(h)/2
	left = math.Max(0, math.Min(left, float64(img.Width()-w)))
	top = math.Max(0, math.Min(top, float64(img.PageHeight()-h)))
	return img.ExtractArea(int(left), int(top), w, h)
}

func (v *Processor) animatedThumbnailWithCrop(
	img *Image, w, h int, crop Interesting, size Size,
) (err error) {
	if size == SizeDown && img.Width() < w && img.PageHeight() < h {
		return
	}
	var top, left int
	if float64(w)/float64(h) > float64(img.Width())/float64(img.PageHeight()) {
		if err = img.ThumbnailWithSize(w, v.MaxHeight, InterestingNone, size); err != nil {
			return
		}
	} else {
		if err = img.ThumbnailWithSize(v.MaxWidth, h, InterestingNone, size); err != nil {
			return
		}
	}
	if crop == InterestingHigh {
		left = img.Width() - w
		top = img.PageHeight() - h
	} else if crop == InterestingCentre || crop == InterestingAttention {
		left = (img.Width() - w) / 2
		top = (img.PageHeight() - h) / 2
	}
	return img.ExtractArea(left, top, w, h)
}

// CheckResolution check image resolution for image bomb prevention
func (v *Processor) CheckResolution(img *Image, err error) (*Image, error) {
	if err != nil || img == nil {
		return img, err
	}
	if img.Width() > v.MaxWidth || img.PageHeight() > v.MaxHeight ||
		(img.Width()*img.Height()) > v.MaxResolution {
		img.Close()
		return nil, imagor.ErrMaxResolutionExceeded
	}
	return img, nil
}

// WrapErr wraps error to become imagor.Error
func WrapErr(err error) error {
	if err == nil {
		return nil
	}
	if e, ok := err.(imagor.Error); ok {
		return e
	}
	msg := strings.TrimSpace(err.Error())
	if strings.HasPrefix(msg, "VipsForeignLoad:") &&
		strings.HasSuffix(msg, "is not in a known format") {
		return imagor.ErrUnsupportedFormat
	}
	return imagor.NewError(msg, 406)
}
