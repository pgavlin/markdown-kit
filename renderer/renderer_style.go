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
	} else if !new.Background.IsSet() && !base.IsZero() && base.Background.IsSet() {
		if err := r.writeSGR(w, "49"); err != nil {
			return err
		}
	}
	if new.Colour.IsSet() && (base.IsZero() || new.Colour != base.Colour) {
		if err := r.writeColorSGR(w, "38", new.Colour); err != nil {
			return err
		}
	} else if !new.Colour.IsSet() && !base.IsZero() && base.Colour.IsSet() {
		if err := r.writeSGR(w, "39"); err != nil {
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

// resolveStyle looks up the token's style in the theme and applies inheritance
// from the current top of the style stack. Returns the resolved style entry and
// true, or a zero entry and false if the theme is nil or has no style for the token.
func (r *Renderer) resolveStyle(token chroma.TokenType) (chroma.StyleEntry, bool) {
	if r.theme == nil {
		return chroma.StyleEntry{}, false
	}

	tokenStyle := r.theme.Get(token)
	if tokenStyle.IsZero() {
		return chroma.StyleEntry{}, false
	}

	var base chroma.StyleEntry
	if len(r.styles) != 0 {
		base = r.styles[len(r.styles)-1]
	}
	// Inherit colors from parent when unset.
	if !tokenStyle.Background.IsSet() && base.Background.IsSet() {
		tokenStyle.Background = base.Background
	}
	if !tokenStyle.Colour.IsSet() && base.Colour.IsSet() {
		tokenStyle.Colour = base.Colour
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
	return tokenStyle, true
}

// reapplyStyle writes the ANSI sequences needed to re-establish the current
// top of the style stack. This is used after an SGR reset (0) to restore the
// active style when inline ANSI sequences from pre-rendered content may have
// left the terminal in an unknown state.
func (r *Renderer) reapplyStyle(w io.Writer) error {
	if len(r.styles) == 0 {
		return nil
	}
	return r.writeDelta(w, chroma.StyleEntry{}, r.styles[len(r.styles)-1])
}

// buildStyleSGR returns the raw ANSI SGR bytes needed to establish the given
// style from a reset state. Returns nil if the style is zero. This is used by
// beginLine to restore the active style after writing the prefix without going
// through the renderer's Write pipeline (which would recurse back into beginLine).
func buildStyleSGR(style chroma.StyleEntry) []byte {
	if style.IsZero() {
		return nil
	}
	var buf []byte
	if style.Background.IsSet() {
		buf = fmt.Appendf(buf, "\033[48;2;%d;%d;%dm", style.Background.Red(), style.Background.Green(), style.Background.Blue())
	}
	if style.Colour.IsSet() {
		buf = fmt.Appendf(buf, "\033[38;2;%d;%d;%dm", style.Colour.Red(), style.Colour.Green(), style.Colour.Blue())
	}
	if style.Bold == chroma.Yes {
		buf = append(buf, "\033[1m"...)
	}
	if style.Underline == chroma.Yes {
		buf = append(buf, "\033[4m"...)
	}
	if style.Italic == chroma.Yes {
		buf = append(buf, "\033[3m"...)
	}
	return buf
}

func (r *Renderer) PushStyle(w io.Writer, token chroma.TokenType) error {
	resolved, ok := r.resolveStyle(token)
	if !ok {
		return nil
	}

	var base chroma.StyleEntry
	if len(r.styles) != 0 {
		base = r.styles[len(r.styles)-1]
	}
	if err := r.writeDelta(w, base, resolved); err != nil {
		return err
	}
	r.styles = append(r.styles, resolved)
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
