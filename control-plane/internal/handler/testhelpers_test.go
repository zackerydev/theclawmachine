package handler

import "io"

// noopRenderer is a TemplateRenderer that does nothing (for unit tests).
type noopRenderer struct{}

func (n *noopRenderer) Render(w io.Writer, name string, data any, isHTMX bool) error {
	return nil
}
