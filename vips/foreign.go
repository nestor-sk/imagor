package vips

// #include "foreign.h"
import "C"

// JpegExportParams are options when exporting a JPEG to file or buffer
type JpegExportParams struct {
	StripMetadata      bool
	Quality            int
	Interlace          bool
	OptimizeCoding     bool
	SubsampleMode      SubsampleMode
	TrellisQuant       bool
	OvershootDeringing bool
	OptimizeScans      bool
	QuantTable         int
}

// NewJpegExportParams creates default values for an export of a JPEG image.
// By default, vips creates interlaced JPEGs with a quality of 80/100.
func NewJpegExportParams() *JpegExportParams {
	return &JpegExportParams{
		Quality:   80,
		Interlace: true,
	}
}

// PngExportParams are options when exporting a PNG to file or buffer
type PngExportParams struct {
	StripMetadata bool
	Compression   int
	Filter        PngFilter
	Interlace     bool
	Quality       int
	Palette       bool
	Dither        float64
	Bitdepth      int
	Profile       string // TODO: Use this param during save
}

// NewPngExportParams creates default values for an export of a PNG image.
// By default, vips creates non-interlaced PNGs with a compression of 6/10.
func NewPngExportParams() *PngExportParams {
	return &PngExportParams{
		Compression: 6,
		Filter:      PngFilterNone,
		Interlace:   false,
		Palette:     false,
	}
}

func vipsSaveJPEGToBuffer(in *C.VipsImage, params JpegExportParams) ([]byte, error) {
	p := C.create_save_params(C.JPEG)
	p.inputImage = in
	p.stripMetadata = C.int(boolToInt(params.StripMetadata))
	p.quality = C.int(params.Quality)
	p.interlace = C.int(boolToInt(params.Interlace))
	p.jpegOptimizeCoding = C.int(boolToInt(params.OptimizeCoding))
	p.jpegSubsample = C.VipsForeignJpegSubsample(params.SubsampleMode)
	p.jpegTrellisQuant = C.int(boolToInt(params.TrellisQuant))
	p.jpegOvershootDeringing = C.int(boolToInt(params.OvershootDeringing))
	p.jpegOptimizeScans = C.int(boolToInt(params.OptimizeScans))
	p.jpegQuantTable = C.int(params.QuantTable)

	return vipsSaveToBuffer(p)
}

func vipsSavePNGToBuffer(in *C.VipsImage, params PngExportParams) ([]byte, error) {
	p := C.create_save_params(C.PNG)
	p.inputImage = in
	p.quality = C.int(params.Quality)
	p.stripMetadata = C.int(boolToInt(params.StripMetadata))
	p.interlace = C.int(boolToInt(params.Interlace))
	p.pngCompression = C.int(params.Compression)
	p.pngFilter = C.VipsForeignPngFilter(params.Filter)
	p.pngPalette = C.int(boolToInt(params.Palette))
	p.pngDither = C.double(params.Dither)
	p.pngBitdepth = C.int(params.Bitdepth)

	return vipsSaveToBuffer(p)
}

func vipsSaveToBuffer(params C.struct_SaveParams) ([]byte, error) {
	if err := C.save_to_buffer(&params); err != 0 {
		if params.outputBuffer != nil {
			gFreePointer(params.outputBuffer)
		}
		return nil, handleVipsError()
	}

	buf := C.GoBytes(params.outputBuffer, C.int(params.outputLen))
	defer gFreePointer(params.outputBuffer)

	return buf, nil
}
