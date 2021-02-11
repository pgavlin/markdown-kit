package renderer

import (
	"fmt"
	"io"

	"github.com/alecthomas/chroma"
)

func (r *Renderer) writeSGR(w io.Writer, command string) error {
	s := fmt.Sprintf("\033[%sm", command)
	_, err := r.Write(w, []byte(s))
	return err
}

func (r *Renderer) writeTristateSGR(w io.Writer, off, on string, value chroma.Trilean) error {
	if value == chroma.Yes {
		return r.writeSGR(w, on)
	}
	return r.writeSGR(w, off)
}

func (r *Renderer) writeColorSGR(w io.Writer, command string, color chroma.Colour) error {
	return r.writeSGR(w, command+fmt.Sprintf(";2;%v;%v;%v", color.Red(), color.Green(), color.Blue()))
}

func (r *Renderer) writeDelta(w io.Writer, base, new chroma.StyleEntry) error {
	if new.IsZero() {
		// Write a reset command.
		return r.writeSGR(w, "0")
	}

	if new.Background.IsSet() && (base.IsZero() || new.Background != base.Background) {
		if err := r.writeColorSGR(w, "48", new.Background); err != nil {
			return err
		}
	}
	if new.Colour.IsSet() && (base.IsZero() || new.Colour != base.Colour) {
		if err := r.writeColorSGR(w, "38", new.Colour); err != nil {
			return err
		}
	}
	if base.IsZero() || new.Bold != base.Bold {
		if err := r.writeTristateSGR(w, "22", "1", new.Bold); err != nil {
			return err
		}
	}
	if base.IsZero() || new.Underline != base.Underline {
		if err := r.writeTristateSGR(w, "24", "4", new.Underline); err != nil {
			return err
		}
	}
	if base.IsZero() || new.Italic != base.Italic {
		if err := r.writeTristateSGR(w, "23", "3", new.Italic); err != nil {
			return err
		}
	}
	return nil
}

func (r *Renderer) PushStyle(w io.Writer, token chroma.TokenType) error {
	if r.theme == nil {
		return nil
	}

	tokenStyle := r.theme.Get(token)
	if tokenStyle.IsZero() {
		return nil
	}

	var base chroma.StyleEntry
	if len(r.styles) != 0 {
		base = r.styles[len(r.styles)-1]
	}
	if tokenStyle.Bold == chroma.Pass {
		tokenStyle.Bold = base.Bold
	}
	if tokenStyle.Underline == chroma.Pass {
		tokenStyle.Underline = base.Underline
	}
	if tokenStyle.Italic == chroma.Pass {
		tokenStyle.Italic = base.Italic
	}
	if err := r.writeDelta(w, base, tokenStyle); err != nil {
		return err
	}
	r.styles = append(r.styles, tokenStyle)
	return nil
}

func (r *Renderer) PopStyle(w io.Writer) error {
	if r.theme == nil {
		return nil
	}

	var new chroma.StyleEntry
	if len(r.styles) > 1 {
		new = r.styles[len(r.styles)-2]
	}
	if err := r.writeDelta(w, r.styles[len(r.styles)-1], new); err != nil {
		return err
	}
	r.styles = r.styles[:len(r.styles)-1]
	return nil
}
