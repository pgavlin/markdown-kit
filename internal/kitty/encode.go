package kitty

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"io"
)

func fprintf(w io.Writer, written *int, f string, args ...interface{}) error {
	n, err := fmt.Fprintf(w, f, args...)
	*written += n
	return err
}

// Encode encodes an image to a Writer using the kitty graphics protocol.
func Encode(w io.Writer, image image.Image) (int, error) {
	var buf bytes.Buffer
	enc := base64.NewEncoder(base64.StdEncoding, &buf)
	if err := png.Encode(enc, image); err != nil {
		return 0, err
	}
	defer enc.Close()
	data := buf.Bytes()

	written := 0
	for len(data) > 0 {
		if written == 0 {
			if err := fprintf(w, &written, "\x1b_Gf=100,a=T,"); err != nil {
				return written, err
			}
		} else {
			if err := fprintf(w, &written, "\x1b_G"); err != nil {
				return written, err
			}
		}

		more, b := 0, data
		if len(data) > 4096 {
			more, b = 1, data[:4096]
		}
		if err := fprintf(w, &written, "m=%d;", more); err != nil {
			return written, err
		}
		n, err := w.Write(b)
		written += n
		if err != nil {
			return written, err
		}
		if err := fprintf(w, &written, "\x1b\\"); err != nil {
			return written, err
		}

		data = data[len(b):]
	}

	return written, nil
}
