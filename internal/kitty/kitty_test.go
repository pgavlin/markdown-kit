package kitty

import (
	"bytes"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestImage returns a width×height RGBA image filled with a solid color.
func newTestImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 0xAA, G: 0xBB, B: 0xCC, A: 0xFF})
		}
	}
	return img
}

// newNoisyImage returns an image with per-pixel variation so its PNG
// encoding doesn't compress below the chunk threshold.
func newNoisyImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x*7 + y*3) & 0xFF),
				G: uint8((x*11 + y*5) & 0xFF),
				B: uint8((x*13 + y*17) & 0xFF),
				A: 0xFF,
			})
		}
	}
	return img
}

func TestEncode_SingleChunk(t *testing.T) {
	// A 2×2 image has a PNG payload small enough to fit in one chunk (< 4096 bytes base64).
	var buf bytes.Buffer
	n, err := Encode(&buf, newTestImage(2, 2))
	require.NoError(t, err)
	assert.Equal(t, n, buf.Len(),
		"returned byte count should equal buffer length")

	out := buf.String()
	// Prefix: \x1b_G with the standard options (format 100 = PNG, action T = transmit+display).
	assert.True(t, strings.HasPrefix(out, "\x1b_Gf=100,a=T,m=0;"),
		"expected encoded prefix, got %q", out[:min(len(out), 40)])
	// Suffix: \x1b\\
	assert.True(t, strings.HasSuffix(out, "\x1b\\"),
		"expected ST terminator, got %q", out[max(0, len(out)-10):])
}

func TestEncode_MultiChunk(t *testing.T) {
	// A 256×256 noisy image's PNG base64 is ~9 KB — 3 chunks of 4096 bytes.
	var buf bytes.Buffer
	_, err := Encode(&buf, newNoisyImage(256, 256))
	require.NoError(t, err)

	out := buf.String()
	// Count the escape-sequence starts: one per chunk.
	starts := strings.Count(out, "\x1b_G")
	terminators := strings.Count(out, "\x1b\\")
	assert.Greater(t, starts, 1,
		"expected multi-chunk output, saw %d chunks: %q", starts, out[:min(60, len(out))])
	assert.Equal(t, starts, terminators,
		"every chunk must be ST-terminated")

	// First chunk carries the option set; subsequent chunks just "\x1b_G m=X;..."
	assert.True(t, strings.HasPrefix(out, "\x1b_Gf=100,a=T,m=1;"),
		"first chunk should have full options and m=1")
	// Last chunk has m=0.
	assert.Contains(t, out, "m=0;",
		"at least one chunk must close the sequence with m=0")
}

func TestDecodeCommand_SingleByteAction(t *testing.T) {
	data := []byte("\x1b_Ga=T;\x1b\\")
	var c Command
	n := DecodeCommand(&c, data)
	assert.Equal(t, len(data), n)
	assert.Equal(t, byte('T'), c.Action)
}

func TestDecodeCommand_MultiDigitFormat(t *testing.T) {
	data := []byte("\x1b_Gf=100,a=T,m=0;\x1b\\")
	var c Command
	n := DecodeCommand(&c, data)
	require.Equal(t, len(data), n, "decode should consume entire frame")
	assert.Equal(t, uint(100), c.Format)
	assert.Equal(t, byte('T'), c.Action)
	assert.False(t, c.More)
}

func TestDecodeCommand_MoreFlag(t *testing.T) {
	data := []byte("\x1b_Gf=100,a=T,m=1;\x1b\\")
	var c Command
	n := DecodeCommand(&c, data)
	require.Equal(t, len(data), n)
	assert.True(t, c.More, "m=1 should set More")
}

func TestDecodeCommand_WithBase64Payload(t *testing.T) {
	// Payload "Hi!" → base64 "SGkh"
	data := []byte("\x1b_Ga=T;SGkh\x1b\\")
	var c Command
	n := DecodeCommand(&c, data)
	require.Equal(t, len(data), n)
	assert.Equal(t, []byte("Hi!"), c.Payload)
}

func TestDecodeCommand_RejectsInvalidPrefix(t *testing.T) {
	for _, bad := range []string{
		"",
		"no-escape",
		"\x1b_X",                      // not _G
		"\x1b[A",                      // CSI, not APC
		"\x1b_Ga=T",                   // missing ST terminator
		"\x1b_Ga",                     // no key=value separator
		"\x1b_Ga=T;",                  // no ST
		"\x1b_Ga=T;not base64;\x1b\\", // payload fails base64 decode
	} {
		var c Command
		n := DecodeCommand(&c, []byte(bad))
		assert.Equal(t, 0, n, "should reject %q", bad)
	}
}

func TestDecodeCommand_IntegerRejectsNonDigit(t *testing.T) {
	data := []byte("\x1b_Gf=10X;\x1b\\")
	var c Command
	n := DecodeCommand(&c, data)
	assert.Equal(t, 0, n, "non-digit in integer field must reject")
}

func TestDecodeCommands_SequenceStopsAtMoreFalse(t *testing.T) {
	// Two commands back-to-back: first has m=1, second has m=0.
	// DecodeCommands should return both and stop.
	data := []byte("\x1b_Gf=100,a=T,m=1;\x1b\\\x1b_Gm=0;\x1b\\")
	cmds, n := DecodeCommands(data)
	require.Equal(t, len(data), n)
	require.Len(t, cmds, 2)
	assert.True(t, cmds[0].More)
	assert.False(t, cmds[1].More)
}

func TestDecodeCommands_StopsOnInvalidFollower(t *testing.T) {
	// First command is valid with More=true, but trailing bytes are junk.
	data := []byte("\x1b_Gf=100,a=T,m=1;\x1b\\garbage")
	cmds, n := DecodeCommands(data)
	require.Len(t, cmds, 1)
	assert.True(t, cmds[0].More)
	// n should account for only the first command's bytes.
	assert.Less(t, n, len(data))
}

func TestEncode_DecodeRoundTrip(t *testing.T) {
	// Encode a small image, then decode every chunk and reassemble the
	// base64 payload. The concatenated payload must successfully re-decode
	// as a PNG header.
	var buf bytes.Buffer
	_, err := Encode(&buf, newTestImage(4, 4))
	require.NoError(t, err)

	cmds, n := DecodeCommands(buf.Bytes())
	require.Equal(t, buf.Len(), n, "decode should consume everything")
	require.NotEmpty(t, cmds)

	// First chunk carries the format; subsequent chunks don't.
	assert.Equal(t, uint(100), cmds[0].Format)
	assert.Equal(t, byte('T'), cmds[0].Action)

	// Reassemble payload.
	var payload []byte
	for _, c := range cmds {
		payload = append(payload, c.Payload...)
	}
	// PNG magic: 89 50 4E 47 0D 0A 1A 0A
	require.Greater(t, len(payload), 8)
	assert.Equal(t, byte(0x89), payload[0])
	assert.Equal(t, "PNG", string(payload[1:4]))
}
