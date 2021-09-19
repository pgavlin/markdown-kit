package kitty

import (
	"bytes"
	"encoding/base64"
)

type Command struct {
	// The overall action this graphics command is performing.
	// t - transmit data
	// T - transmit data and display image
	// q - query terminal
	// p - put (display) previous transmitted image
	// d - delete image
	// f - transmit data for animation frames
	// a - control animation
	// c - compose animation frames
	Action byte

	// Suppress responses from the terminal to this graphics command.
	// 0 - do not suppress responses.
	// 1 - suppress OK responses.
	// 2 - suppress failure responses.
	Quiet byte

	// The format in which the image data is sent.
	// 24 - RGB pixel data
	// 32 - RGBA pixel data
	// 100 - PNG image data
	Format uint

	// The transmission medium used.
	// d - Direct (the data is transmitted within the escape code itself)
	// f - A simple file
	// t - A temporary file, the terminal emulator will delete the file after reading
	//     the pixel data. For security reasons the terminal emulator should only delete
	//     the file if it is in a known temporary directory, such as /tmp, /dev/shm,
	//     TMPDIR env var if present and any platform specific temporary directories.
	// s - A shared memory object, which on POSIX systems is a POSIX shared memory object
	//     and on Windows is a Named shared memory object. The terminal emulator must read
	//     the data from the memory object and then unlink and close it on POSIX and just
	//     close it on Windows.
	Medium byte

	// The width of the image being sent.
	Width uint

	// The height of the image being sent.
	Height uint

	// The size of data to read from a file.
	Size uint

	// The offset from which to read data from a file.
	Offset uint

	// The image id
	ID uint

	// The image number
	Number uint

	// The placement id
	Placement uint

	// The type of data compression.
	// 0 - no compression
	// z - RFC 1950 ZLIB based deflate compression
	Compression byte

	// Whether there is more chunked data available.
	More bool

	// Payload data.
	Payload []byte
}

func DecodeCommand(c *Command, b []byte) int {
	sz := 0
	if len(b) < 5 || b[0] != 0x1b || b[1] != '_' || b[2] != 'G' {
		return 0
	}
	b, sz = b[3:], 3

	*c = Command{}

	// decode control data
	for len(b) > 0 {
		k := b[0]
		b, sz = b[1:], sz+1

		if k == ';' {
			break
		}

		if len(b) < 2 || b[0] != '=' {
			return 0
		}
		b, sz = b[1:], sz+1

		// find the extent of the value
		v := b[:0]
		for len(b) > 0 {
			if b[0] == ',' {
				b, sz = b[1:], sz+1
				break
			}
			if b[0] == ';' {
				// this byte is accounted for in the next go-round
				break
			}
			v, b, sz = v[:1], b[1:], sz+1
		}

		// decode the value
		var decoder func([]byte) bool
		switch k {
		case 'a':
			decoder = singleCharacterDecoder(&c.Action)
		case 'q':
			decoder = singleCharacterDecoder(&c.Quiet)
		case 'f':
			decoder = positiveIntegerDecoder(&c.Format)
		case 't':
			decoder = singleCharacterDecoder(&c.Quiet)
		case 's':
			decoder = positiveIntegerDecoder(&c.Width)
		case 'v':
			decoder = positiveIntegerDecoder(&c.Height)
		case 'S':
			decoder = positiveIntegerDecoder(&c.Size)
		case 'O':
			decoder = positiveIntegerDecoder(&c.Offset)
		case 'i':
			decoder = positiveIntegerDecoder(&c.ID)
		case 'I':
			decoder = positiveIntegerDecoder(&c.Number)
		case 'p':
			decoder = positiveIntegerDecoder(&c.Placement)
		case 'o':
			decoder = singleCharacterDecoder(&c.Compression)
		case 'm':
			decoder = boolDecoder(&c.More)
		}
		if !decoder(v) {
			return 0
		}
	}

	// decode the payload
	terminator := bytes.Index(b, []byte{0x1b, '\\'})
	if terminator == -1 {
		return 0
	}
	sz = sz + terminator + 2

	base64Payload := b[:terminator]
	payloadSize := base64.StdEncoding.DecodedLen(len(base64Payload))
	payload := make([]byte, payloadSize)
	if n, err := base64.StdEncoding.Decode(payload, base64Payload); err != nil || n != payloadSize {
		return 0
	}
	c.Payload = payload

	return sz
}

func DecodeCommands(b []byte) ([]Command, int) {
	var commands []Command
	var size int
	for {
		var c Command
		sz := DecodeCommand(&c, b)
		if sz == 0 {
			return commands, size
		}
		commands, b, size = append(commands, c), b[sz:], size+sz
		if !c.More {
			return commands, size
		}
	}
}

func boolDecoder(dest *bool) func([]byte) bool {
	return func(b []byte) bool {
		if len(b) < 1 {
			return false
		}
		*dest = b[0] != '0'
		return true
	}
}

func singleCharacterDecoder(dest *byte) func([]byte) bool {
	return func(b []byte) bool {
		if len(b) < 1 {
			return false
		}
		*dest = b[0]
		return true
	}
}

func positiveIntegerDecoder(dest *uint) func([]byte) bool {
	return func(b []byte) bool {
		val, any := uint(0), false
		for len(b) > 0 {
			c := b[0]
			if c < '0' || c > '9' {
				return false
			}
			val, any = val*10+uint(c-'0'), true
		}
		if !any {
			return false
		}
		*dest = val
		return true
	}
}
