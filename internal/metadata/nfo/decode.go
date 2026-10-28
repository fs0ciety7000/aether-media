package nfo

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// node is a lenient, lower-cased XML element tree.
type node struct {
	name     string // lower-cased local name
	attrs    map[string]string
	text     strings.Builder
	children []*node
}

func (n *node) Text() string { return strings.TrimSpace(n.text.String()) }

func (n *node) Attr(name string) string { return strings.TrimSpace(n.attrs[name]) }

// child returns the first child named name, nil if none.
func (n *node) child(name string) *node {
	for _, c := range n.children {
		if c.name == name {
			return c
		}
	}
	return nil
}

// childText returns the text of the first non-empty child among names.
func (n *node) childText(names ...string) string {
	for _, name := range names {
		for _, c := range n.children {
			if c.name == name {
				if t := c.Text(); t != "" {
					return t
				}
			}
		}
	}
	return ""
}

// all returns every child named name.
func (n *node) all(name string) []*node {
	var out []*node
	for _, c := range n.children {
		if c.name == name {
			out = append(out, c)
		}
	}
	return out
}

// document is a decoded NFO: its top-level elements (several for
// multi-episode files) and the text found outside of them (a trailing
// IMDb/TMDb URL, or the whole file for URL-only NFOs).
type document struct {
	roots   []*node
	outside string
}

// maxNFOSize bounds what is read from one NFO file.
const maxNFOSize = 8 << 20

// toUTF8 strips byte-order marks and converts UTF-16 and invalid UTF-8
// (assumed Windows-1252 / Latin-1) to UTF-8, whatever the XML declaration
// says.
func toUTF8(b []byte) []byte {
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		b = b[3:]
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		return decodeUTF16(b[2:], false)
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		return decodeUTF16(b[2:], true)
	}
	if utf8.Valid(b) {
		return b
	}
	var buf bytes.Buffer
	buf.Grow(len(b) + len(b)/8)
	for _, c := range b {
		if c >= 0x80 && c <= 0x9F {
			if r := cp1252[c-0x80]; r != 0 {
				buf.WriteRune(r)
				continue
			}
		}
		buf.WriteRune(rune(c))
	}
	return buf.Bytes()
}

func decodeUTF16(b []byte, bigEndian bool) []byte {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		if bigEndian {
			u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
		} else {
			u = append(u, uint16(b[i+1])<<8|uint16(b[i]))
		}
	}
	return []byte(string(utf16.Decode(u)))
}

// cp1252 maps the 0x80-0x9F range of Windows-1252 (the usual "Latin-1" of
// Windows tools) to runes; 0 = undefined, kept as the C1 control.
var cp1252 = [32]rune{
	'€', 0, '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', 0, 'Ž', 0,
	0, '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', 0, 'ž', 'Ÿ',
}

// decode parses data leniently: non-strict mode, HTML entities, any declared
// charset (data is already UTF-8), mismatched tags auto-closed, and anything
// after a broken element ignored. It fails only when the content cannot be
// read at all.
func decode(r io.Reader) (*document, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxNFOSize))
	if err != nil {
		return nil, err
	}
	data := toUTF8(raw)
	doc := &document{}
	if !bytes.Contains(data, []byte("<")) {
		doc.outside = string(data)
		return doc, nil
	}

	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity
	d.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }

	var outside strings.Builder
	var stack []*node
	for {
		tok, err := d.Token()
		if err != nil {
			if !errors.Is(err, io.EOF) && len(doc.roots) == 0 {
				// Nothing parsed: maybe "http://... <garbage>"; keep the text
				// for id extraction.
				outside.Write(data)
			}
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{name: strings.ToLower(t.Name.Local)}
			for _, a := range t.Attr {
				if n.attrs == nil {
					n.attrs = make(map[string]string, len(t.Attr))
				}
				n.attrs[strings.ToLower(a.Name.Local)] = a.Value
			}
			if len(stack) == 0 {
				doc.roots = append(doc.roots, n)
			} else {
				p := stack[len(stack)-1]
				p.children = append(p.children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) == 0 {
				outside.Write(t)
				outside.WriteByte('\n')
			} else {
				stack[len(stack)-1].text.Write(t)
			}
		}
	}
	doc.outside = outside.String()
	return doc, nil
}
