package cloudatlas

import (
	"encoding/json"
	"fmt"
	"io"
)

type Format string

const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
)

type Renderer struct {
	format Format
	out    io.Writer
}

func NewRenderer(format string, out io.Writer) Renderer {
	if format == string(FormatJSON) {
		return Renderer{format: FormatJSON, out: out}
	}
	return Renderer{format: FormatTable, out: out}
}

func (r Renderer) Render(data json.RawMessage) error {
	if len(data) == 0 || string(data) == "null" {
		_, err := fmt.Fprintln(r.out, "null")
		return err
	}

	if r.format == FormatJSON {
		return r.renderJSON(data)
	}

	var list ListEnvelope
	if err := json.Unmarshal(data, &list); err == nil && len(list.Items) > 0 {
		if err := r.renderPretty(list.Items); err != nil {
			return err
		}
		_, err := fmt.Fprintf(r.out, "total=%d current=%d size=%d\n", list.Total, list.Current, list.Size)
		return err
	}

	return r.renderPretty(data)
}

func (r Renderer) renderJSON(data json.RawMessage) error {
	_, err := fmt.Fprintln(r.out, string(data))
	return err
}

func (r Renderer) renderPretty(data json.RawMessage) error {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("parse response for render: %w", err)
	}
	pretty, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("render response: %w", err)
	}
	_, err = fmt.Fprintln(r.out, string(pretty))
	return err
}
