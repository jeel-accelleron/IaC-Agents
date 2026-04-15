package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ghcp-iac/ghcp-iac-workflow/internal/protocol"
)

// PlainWriter collects all agent output and writes it as a single plain-text HTTP response.
// It is used when the client sends Accept: text/plain or the ?format=plain query parameter.
type PlainWriter struct {
	w   http.ResponseWriter
	buf strings.Builder
}

// NewPlainWriter creates a PlainWriter backed by the given response writer.
func NewPlainWriter(w http.ResponseWriter) *PlainWriter {
	return &PlainWriter{w: w}
}

// SendMessage appends content to the internal buffer.
func (p *PlainWriter) SendMessage(content string) {
	p.buf.WriteString(content)
}

// SendReferences appends a References section to the buffer.
func (p *PlainWriter) SendReferences(refs []protocol.Reference) {
	if len(refs) == 0 {
		return
	}
	p.buf.WriteString("\n### References\n")
	for _, r := range refs {
		fmt.Fprintf(&p.buf, "- [%s](%s)\n", r.Title, r.URL)
	}
}

// SendConfirmation appends a confirmation notice to the buffer.
func (p *PlainWriter) SendConfirmation(conf protocol.Confirmation) {
	fmt.Fprintf(&p.buf, "\n**%s**: %s\n", conf.Title, conf.Message)
}

// SendError appends an error message to the buffer.
func (p *PlainWriter) SendError(msg string) {
	fmt.Fprintf(&p.buf, "\n**Error:** %s\n", msg)
}

// SendDone flushes the accumulated buffer as a plain-text HTTP response.
func (p *PlainWriter) SendDone() {
	p.w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(p.w, p.buf.String())
}
