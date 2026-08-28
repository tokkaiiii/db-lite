package query

import "fmt"

// CellFile is one column value shaped for a raw file download response
// (ADR 0009): the exact bytes to write, plus the Content-Type and file
// extension to send alongside them.
type CellFile struct {
	Data        []byte
	ContentType string
	Extension   string
}

// NewCellFile classifies value by its Go type alone — []byte is binary,
// everything else is rendered as text — deliberately not inspecting the
// content to guess a more specific format (XML, JSON, ...): ADR 0009
// scopes this to a plain download, not a format-aware viewer.
func NewCellFile(value any) CellFile {
	switch v := value.(type) {
	case []byte:
		return CellFile{Data: v, ContentType: "application/octet-stream", Extension: ".bin"}
	case nil:
		return CellFile{Data: nil, ContentType: "text/plain; charset=utf-8", Extension: ".txt"}
	case string:
		return CellFile{Data: []byte(v), ContentType: "text/plain; charset=utf-8", Extension: ".txt"}
	default:
		return CellFile{Data: []byte(fmt.Sprintf("%v", v)), ContentType: "text/plain; charset=utf-8", Extension: ".txt"}
	}
}
