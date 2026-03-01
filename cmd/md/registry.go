package main

import "strings"

// converterRegistry maps file extensions and MIME types to converters.
type converterRegistry struct {
	byExtension map[string][]converter
	byMIMEType  map[string][]converter
	extensions  []string
}

// newConverterRegistry builds a registry from format converter configs.
// Configs are processed in order; when multiple configs register the same
// extension or MIME type the resulting converter tries them in config order.
func newConverterRegistry(configs []formatConverterConfig, shell shellRunner) *converterRegistry {
	if len(configs) == 0 {
		return nil
	}

	r := &converterRegistry{
		byExtension: make(map[string][]converter),
		byMIMEType:  make(map[string][]converter),
	}

	seen := make(map[string]bool)
	for _, cfg := range configs {
		conv := &externalConverter{command: cfg.Command, shell: shell}
		for _, ext := range cfg.Extensions {
			ext = strings.ToLower(ext)
			r.byExtension[ext] = append(r.byExtension[ext], conv)
			if !seen[ext] {
				seen[ext] = true
				r.extensions = append(r.extensions, ext)
			}
		}
		for _, mt := range cfg.MIMETypes {
			mt = strings.ToLower(mt)
			r.byMIMEType[mt] = append(r.byMIMEType[mt], conv)
		}
	}

	return r
}

// converterFromSlice returns a single converter for a slice of candidates.
// Returns nil for an empty slice, the sole element for length 1, or a
// fallbackConverter that tries each in order.
func converterFromSlice(convs []converter) converter {
	switch len(convs) {
	case 0:
		return nil
	case 1:
		return convs[0]
	default:
		return &fallbackConverter{converters: convs}
	}
}

// forExtension returns the converter for the given file extension, or nil.
// When multiple converters are registered they are tried in config order.
func (r *converterRegistry) forExtension(ext string) converter {
	if r == nil {
		return nil
	}
	return converterFromSlice(r.byExtension[strings.ToLower(ext)])
}

// forMIMEType returns the converter for the given MIME type, stripping
// any parameters (e.g. charset). Returns nil if no match. When multiple
// converters are registered they are tried in config order.
func (r *converterRegistry) forMIMEType(mimeType string) converter {
	if r == nil {
		return nil
	}
	// Strip parameters (e.g. "text/x-rst; charset=utf-8" → "text/x-rst").
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}
	return converterFromSlice(r.byMIMEType[strings.ToLower(mimeType)])
}

// allExtensions returns all file extensions the registry handles.
func (r *converterRegistry) allExtensions() []string {
	if r == nil {
		return nil
	}
	return r.extensions
}
