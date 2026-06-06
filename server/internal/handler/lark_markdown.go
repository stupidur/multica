package handler

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	gfmast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// larkMarkdownParser is the package-level goldmark parser. goldmark is safe
// for concurrent use after construction, so a single shared instance is fine.
var larkMarkdownParser = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
)

// renderMarkdownToLarkElements parses a markdown source string and emits a
// list of Feishu card elements. The previous single-lark_md-div approach
// could not render features lark_md does not natively understand (tables,
// fenced code blocks, horizontal rules, headings). Walking the AST lets
// those become proper Feishu card elements.
//
// Supported input:
//   - paragraphs (inline markdown via lark_md: **bold** *italic* ~~strike~~
//     `code` [text](url))
//   - headings (rendered as bold lark_md — lark_md has no heading syntax)
//   - fenced / indented code blocks → pre element
//   - GFM tables → table element
//   - bullet / ordered lists → bulleted lark_md
//   - horizontal rules → hr
//   - blockquotes → lark_md
//
// Returns nil when the source has no content to render.
func renderMarkdownToLarkElements(source string) []map[string]any {
	if strings.TrimSpace(source) == "" {
		return nil
	}
	src := []byte(source)
	root := larkMarkdownParser.Parser().Parse(text.NewReader(src))

	var elements []map[string]any
	for child := root.FirstChild(); child != nil; child = child.NextSibling() {
		if el := renderMarkdownBlock(child, src); el != nil {
			elements = append(elements, el...)
		}
	}
	if len(elements) == 0 {
		return nil
	}
	return elements
}

// renderMarkdownBlock converts a top-level AST block node into one or more
// Feishu card elements. Returns nil when the block yields no content.
func renderMarkdownBlock(node ast.Node, source []byte) []map[string]any {
	switch n := node.(type) {
	case *ast.Paragraph:
		text := renderMarkdownInline(n, source)
		if text == "" {
			return nil
		}
		return larkMDDiv(text)

	case *ast.Heading:
		text := renderMarkdownInline(n, source)
		if text == "" {
			return nil
		}
		// lark_md has no heading syntax; bold the text and emit a div.
		return larkMDDiv("**" + text + "**")

	case *ast.FencedCodeBlock:
		lang := ""
		if n.Info != nil {
			lang = string(n.Info.Segment.Value(source))
		}
		return []map[string]any{renderCodeBlock(lang, n.Lines(), source)}

	case *ast.CodeBlock:
		return []map[string]any{renderCodeBlock("", n.Lines(), source)}

	case *ast.Blockquote:
		var buf bytes.Buffer
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if p, ok := c.(*ast.Paragraph); ok {
				if buf.Len() > 0 {
					buf.WriteByte('\n')
				}
				buf.WriteString(renderMarkdownInline(p, source))
			}
		}
		text := strings.TrimSpace(buf.String())
		if text == "" {
			return nil
		}
		return larkMDDiv(text)

	case *ast.List:
		return []map[string]any{renderList(n, source)}

	case *ast.ThematicBreak:
		return []map[string]any{{"tag": "hr"}}

	case *gfmast.Table:
		return []map[string]any{renderTable(n, source)}

	case *ast.HTMLBlock, *ast.TextBlock:
		// raw HTML and stray text blocks produce nothing
		return nil

	default:
		text := renderMarkdownInline(node, source)
		if text == "" {
			return nil
		}
		return larkMDDiv(text)
	}
}

// larkMDDiv wraps a lark_md content string in the standard div+text envelope.
// The content is expected to be already HTML-escaped (renderInlineNode does
// that) so this wrapper just packs it into the card element shape.
func larkMDDiv(content string) []map[string]any {
	return []map[string]any{{
		"tag": "div",
		"text": map[string]any{
			"tag":     "lark_md",
			"content": content,
		},
	}}
}

func renderCodeBlock(language string, lines *text.Segments, source []byte) map[string]any {
	var content bytes.Buffer
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		content.Write(seg.Value(source))
	}
	return map[string]any{
		"tag":      "pre",
		"language": language,
		"content": map[string]any{
			"tag":     "plain_text",
			"content": strings.TrimRight(content.String(), "\n"),
		},
	}
}

func renderList(n *ast.List, source []byte) map[string]any {
	var buf bytes.Buffer
	idx := n.Start
	for item := n.FirstChild(); item != nil; item = item.NextSibling() {
		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		if n.IsOrdered() {
			buf.WriteString(itoa(idx))
			buf.WriteString(". ")
			idx++
		} else {
			buf.WriteString("- ")
		}
		for c := item.FirstChild(); c != nil; c = c.NextSibling() {
			switch cn := c.(type) {
			case *ast.Paragraph, *ast.TextBlock:
				// *ast.TextBlock is the simple single-line list-item body;
				// *ast.Paragraph shows up for multi-line or "loose" items.
				buf.WriteString(renderMarkdownInline(cn, source))
			case *ast.List:
				nested := renderList(cn, source)
				if t, ok := nested["text"].(map[string]any); ok {
					if content, ok := t["content"].(string); ok {
						buf.WriteByte('\n')
						for _, line := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
							buf.WriteString("    ")
							buf.WriteString(line)
							buf.WriteByte('\n')
						}
					}
				}
			}
		}
	}
	return map[string]any{
		"tag": "div",
		"text": map[string]any{
			"tag":     "lark_md",
			"content": buf.String(),
		},
	}
}

func renderTable(n *gfmast.Table, source []byte) map[string]any {
	var header []string
	var rows []map[string]any
	var columns []map[string]any

	for row := n.FirstChild(); row != nil; row = row.NextSibling() {
		var cells []string
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			text := renderMarkdownInline(cell, source)
			cells = append(cells, text)
		}

		if _, isHeader := row.(*gfmast.TableHeader); isHeader {
			header = cells
			for i, title := range cells {
				columns = append(columns, map[string]any{
					"name":             tableColumnName(i),
					"display_name":     title,
					"data_type":        "lark_md",
					"horizontal_align": "left",
					"vertical_align":   "top",
					"width":            "auto",
				})
			}
			continue
		}

		rowData := map[string]any{}
		for i, value := range cells {
			rowData[tableColumnName(i)] = value
		}
		rows = append(rows, rowData)
	}

	if len(columns) == 0 && len(header) == 0 {
		return map[string]any{
			"tag": "div",
			"text": map[string]any{
				"tag":     "lark_md",
				"content": "",
			},
		}
	}

	return map[string]any{
		"tag":       "table",
		"page_size": 10,
		"row_height": "low",
		"header_style": map[string]any{
			"text_align":       "left",
			"text_size":        "normal",
			"background_style": "none",
			"text_color":       "grey",
			"bold":             true,
			"lines":            1,
		},
		"columns": columns,
		"rows":    rows,
	}
}

func tableColumnName(i int) string {
	return "col_" + itoa(i)
}

// renderMarkdownInline walks an AST node's inline children and produces a
// lark_md-compatible content string. Trailing whitespace is trimmed so
// headings and paragraphs surface clean text.
func renderMarkdownInline(node ast.Node, source []byte) string {
	var buf bytes.Buffer
	for c := node.FirstChild(); c != nil; c = c.NextSibling() {
		renderInlineNode(&buf, c, source)
	}
	return strings.TrimSpace(buf.String())
}

func renderInlineNode(buf *bytes.Buffer, node ast.Node, source []byte) {
	if node == nil {
		return
	}
	switch n := node.(type) {
	case *ast.Text:
		buf.Write(escapeLarkTextBytes(n.Segment.Value(source)))
		if n.HardLineBreak() {
			buf.WriteByte('\n')
		}
	case *ast.String:
		buf.WriteString(string(n.Value))
	case *ast.CodeSpan:
		buf.WriteByte('`')
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				buf.Write(t.Segment.Value(source))
			}
		}
		buf.WriteByte('`')
	case *ast.Emphasis:
		if n.Level >= 2 {
			buf.WriteString("**")
		} else {
			buf.WriteByte('*')
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			renderInlineNode(buf, c, source)
		}
		if n.Level >= 2 {
			buf.WriteString("**")
		} else {
			buf.WriteByte('*')
		}
	case *gfmast.Strikethrough:
		buf.WriteString("~~")
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			renderInlineNode(buf, c, source)
		}
		buf.WriteString("~~")
	case *ast.Link:
		buf.WriteByte('[')
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			renderInlineNode(buf, c, source)
		}
		buf.WriteString("](")
		buf.WriteString(string(n.Destination))
		buf.WriteByte(')')
	case *ast.AutoLink:
		url := string(n.URL(source))
		buf.WriteByte('<')
		buf.WriteString(url)
		buf.WriteByte('>')
	case *ast.RawHTML:
		// Render raw HTML as escaped text so it cannot smuggle lark_md tags
		// back into the rendered content. Goldmark's default parser accepts
		// raw HTML, and a comment body that contains <script> would
		// otherwise reach the card verbatim.
		if n.Segments != nil {
			for i := 0; i < n.Segments.Len(); i++ {
				seg := n.Segments.At(i)
				buf.Write(escapeLarkTextBytes(seg.Value(source)))
			}
		}
	default:
		for c := node.FirstChild(); c != nil; c = c.NextSibling() {
			renderInlineNode(buf, c, source)
		}
	}
}

// escapeLarkText escapes the characters that the lark_md renderer would
// interpret as HTML or entity references. Markdown syntax (**, *, ~~, `,
// [, ], (, )) is left intact because lark_md renders it as formatting.
func escapeLarkText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// escapeLarkTextBytes is the []byte variant for use with goldmark segment
// values, which come back as bytes.
func escapeLarkTextBytes(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	out := make([]byte, 0, len(b))
	for _, c := range b {
		switch c {
		case '&':
			out = append(out, '&', 'a', 'm', 'p', ';')
		case '<':
			out = append(out, '&', 'l', 't', ';')
		case '>':
			out = append(out, '&', 'g', 't', ';')
		default:
			out = append(out, c)
		}
	}
	return out
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var s []byte
	for i > 0 {
		s = append([]byte{byte('0' + i%10)}, s...)
		i /= 10
	}
	return string(s)
}
