package tools

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"math"

	"golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	imageTypeSniffBytes = 4100
	maxImageWidth       = 2000
	maxImageHeight      = 2000
	maxImageBase64Bytes = 4_718_592
)

var errImageConversion = errors.New("image conversion failed")

type processedImage struct {
	data           string
	mimeType       string
	originalWidth  int
	originalHeight int
	width          int
	height         int
	convertedFrom  string
}

func processImage(data []byte, mimeType string) (*processedImage, error) {
	normalizedData := data
	normalizedMIME := mimeType
	convertedFrom := ""
	if mimeType == "image/bmp" {
		decoded, err := bmp.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errImageConversion, err)
		}
		var converted bytes.Buffer
		if err := png.Encode(&converted, decoded); err != nil {
			return nil, fmt.Errorf("%w: %v", errImageConversion, err)
		}
		normalizedData = converted.Bytes()
		normalizedMIME = "image/png"
		convertedFrom = mimeType
	}

	config, _, err := image.DecodeConfig(bytes.NewReader(normalizedData))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		if err == nil {
			err = fmt.Errorf("invalid image dimensions")
		}
		return nil, err
	}
	result := &processedImage{
		mimeType:       normalizedMIME,
		originalWidth:  config.Width,
		originalHeight: config.Height,
		width:          config.Width,
		height:         config.Height,
		convertedFrom:  convertedFrom,
	}
	if config.Width <= maxImageWidth &&
		config.Height <= maxImageHeight &&
		base64EncodedLength(len(normalizedData)) < maxImageBase64Bytes {
		result.data = base64.StdEncoding.EncodeToString(normalizedData)
		return result, nil
	}

	source, _, err := image.Decode(bytes.NewReader(normalizedData))
	if err != nil {
		return nil, err
	}
	width, height := constrainedImageDimensions(config.Width, config.Height)
	qualities := []int{80, 85, 70, 55, 40}
	for {
		destination := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.CatmullRom.Scale(destination, destination.Bounds(), source, source.Bounds(), draw.Over, nil)

		var encoded bytes.Buffer
		if err := png.Encode(&encoded, destination); err == nil &&
			base64EncodedLength(encoded.Len()) < maxImageBase64Bytes {
			result.data = base64.StdEncoding.EncodeToString(encoded.Bytes())
			result.mimeType = "image/png"
			result.width = width
			result.height = height
			return result, nil
		}
		for _, quality := range qualities {
			encoded.Reset()
			if err := jpeg.Encode(&encoded, destination, &jpeg.Options{Quality: quality}); err == nil &&
				base64EncodedLength(encoded.Len()) < maxImageBase64Bytes {
				result.data = base64.StdEncoding.EncodeToString(encoded.Bytes())
				result.mimeType = "image/jpeg"
				result.width = width
				result.height = height
				return result, nil
			}
		}

		if width == 1 && height == 1 {
			return nil, fmt.Errorf("image cannot be resized below the inline image size limit")
		}
		nextWidth := width
		nextHeight := height
		if width > 1 {
			nextWidth = max(1, int(math.Floor(float64(width)*0.75)))
		}
		if height > 1 {
			nextHeight = max(1, int(math.Floor(float64(height)*0.75)))
		}
		if nextWidth == width && nextHeight == height {
			return nil, fmt.Errorf("image cannot be resized below the inline image size limit")
		}
		width = nextWidth
		height = nextHeight
	}
}

func constrainedImageDimensions(width int, height int) (int, int) {
	if width > maxImageWidth {
		height = int(math.Round(float64(height) * maxImageWidth / float64(width)))
		width = maxImageWidth
	}
	if height > maxImageHeight {
		width = int(math.Round(float64(width) * maxImageHeight / float64(height)))
		height = maxImageHeight
	}
	return max(1, width), max(1, height)
}

func base64EncodedLength(bytes int) int {
	return int(math.Ceil(float64(bytes)/3)) * 4
}

func detectSupportedImageMIME(data []byte) string {
	if len(data) > imageTypeSniffBytes {
		data = data[:imageTypeSniffBytes]
	}
	switch {
	case len(data) >= 4 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		if data[3] == 0xf7 {
			return ""
		}
		return "image/jpeg"
	case isSupportedPNG(data):
		return "image/png"
	case len(data) >= 3 && string(data[:3]) == "GIF":
		return "image/gif"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp"
	case isSupportedBMP(data):
		return "image/bmp"
	default:
		return ""
	}
}

func isSupportedPNG(data []byte) bool {
	if len(data) < 16 || string(data[:8]) != "\x89PNG\r\n\x1a\n" ||
		binary.BigEndian.Uint32(data[8:12]) != 13 || string(data[12:16]) != "IHDR" {
		return false
	}
	for offset := 8; offset+8 <= len(data); {
		length := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		if length < 0 || offset+12+length > len(data) {
			return true
		}
		chunkType := string(data[offset+4 : offset+8])
		if chunkType == "acTL" {
			return false
		}
		if chunkType == "IDAT" {
			return true
		}
		offset += 12 + length
	}
	return true
}

func isSupportedBMP(data []byte) bool {
	if len(data) < 26 || string(data[:2]) != "BM" {
		return false
	}
	declaredSize := binary.LittleEndian.Uint32(data[2:6])
	pixelOffset := binary.LittleEndian.Uint32(data[10:14])
	headerSize := binary.LittleEndian.Uint32(data[14:18])
	if declaredSize != 0 && declaredSize < 26 {
		return false
	}
	if pixelOffset < 14+headerSize || declaredSize != 0 && pixelOffset >= declaredSize {
		return false
	}
	var planes uint16
	var bitsPerPixel uint16
	switch {
	case headerSize == 12:
		planes = binary.LittleEndian.Uint16(data[22:24])
		bitsPerPixel = binary.LittleEndian.Uint16(data[24:26])
	case headerSize >= 40 && headerSize <= 124:
		if len(data) < 30 {
			return false
		}
		planes = binary.LittleEndian.Uint16(data[26:28])
		bitsPerPixel = binary.LittleEndian.Uint16(data[28:30])
	default:
		return false
	}
	if planes != 1 {
		return false
	}
	for _, supported := range []uint16{1, 4, 8, 16, 24, 32} {
		if bitsPerPixel == supported {
			return true
		}
	}
	return false
}
