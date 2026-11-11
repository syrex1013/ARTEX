package report

import "encoding/json"

// RenderJSON renders a Document as indented JSON.
func RenderJSON(doc Document) ([]byte, error) {
	return json.MarshalIndent(doc, "", "  ")
}
