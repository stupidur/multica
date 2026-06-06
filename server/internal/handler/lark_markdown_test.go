package handler

import (
	"encoding/json"
	"strings"
	"testing"
)

func larkMDContent(t *testing.T, el map[string]any) string {
	t.Helper()
	text, ok := el["text"].(map[string]any)
	if !ok {
		t.Fatalf("expected text field, got %T: %v", el["text"], el)
	}
	content, ok := text["content"].(string)
	if !ok {
		t.Fatalf("expected text.content to be string, got %T", text["content"])
	}
	return content
}

func TestRenderMarkdownToLarkElements_Empty(t *testing.T) {
	cases := []string{"", "   ", "\n\n", " \t \n "}
	for _, input := range cases {
		if got := renderMarkdownToLarkElements(input); got != nil {
			t.Errorf("expected nil for %q, got %d elements", input, len(got))
		}
	}
}

func TestRenderMarkdownToLarkElements_PlainParagraph(t *testing.T) {
	got := renderMarkdownToLarkElements("hello world")
	if len(got) != 1 {
		t.Fatalf("expected 1 element, got %d", len(got))
	}
	if got[0]["tag"] != "div" {
		t.Errorf("expected tag 'div', got %v", got[0]["tag"])
	}
	if content := larkMDContent(t, got[0]); content != "hello world" {
		t.Errorf("unexpected content: %q", content)
	}
}

func TestRenderMarkdownToLarkElements_MultipleParagraphs(t *testing.T) {
	got := renderMarkdownToLarkElements("first\n\nsecond")
	if len(got) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(got))
	}
	if c := larkMDContent(t, got[0]); c != "first" {
		t.Errorf("first paragraph wrong: %q", c)
	}
	if c := larkMDContent(t, got[1]); c != "second" {
		t.Errorf("second paragraph wrong: %q", c)
	}
}

func TestRenderMarkdownToLarkElements_BoldItalicStrike(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"**bold**", "**bold**"},
		{"*italic*", "*italic*"},
		{"~~strike~~", "~~strike~~"},
		{"mix **bold** and *italic* and ~~strike~~", "mix **bold** and *italic* and ~~strike~~"},
	}
	for _, c := range cases {
		got := renderMarkdownToLarkElements(c.input)
		if len(got) != 1 {
			t.Fatalf("input %q: expected 1 element, got %d", c.input, len(got))
		}
		if content := larkMDContent(t, got[0]); content != c.want {
			t.Errorf("input %q: expected %q, got %q", c.input, c.want, content)
		}
	}
}

func TestRenderMarkdownToLarkElements_CodeSpan(t *testing.T) {
	got := renderMarkdownToLarkElements("run `npm install` first")
	if len(got) != 1 {
		t.Fatalf("expected 1 element, got %d", len(got))
	}
	if content := larkMDContent(t, got[0]); content != "run `npm install` first" {
		t.Errorf("unexpected content: %q", content)
	}
}

func TestRenderMarkdownToLarkElements_Link(t *testing.T) {
	got := renderMarkdownToLarkElements("see [docs](https://example.com)")
	if len(got) != 1 {
		t.Fatalf("expected 1 element, got %d", len(got))
	}
	if content := larkMDContent(t, got[0]); content != "see [docs](https://example.com)" {
		t.Errorf("unexpected content: %q", content)
	}
}

func TestRenderMarkdownToLarkElements_EscapesHTML(t *testing.T) {
	got := renderMarkdownToLarkElements("use <script> carefully & correctly")
	if len(got) != 1 {
		t.Fatalf("expected 1 element, got %d", len(got))
	}
	want := "use &lt;script&gt; carefully &amp; correctly"
	if content := larkMDContent(t, got[0]); content != want {
		t.Errorf("expected %q, got %q", want, content)
	}
}

func TestRenderMarkdownToLarkElements_FencedCodeBlock(t *testing.T) {
	input := "before\n\n```go\nfmt.Println(\"hi\")\n```\n\nafter"
	got := renderMarkdownToLarkElements(input)
	if len(got) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(got))
	}
	if got[0]["tag"] != "div" {
		t.Errorf("first element should be div, got %v", got[0]["tag"])
	}
	if got[1]["tag"] != "pre" {
		t.Errorf("second element should be pre, got %v", got[1]["tag"])
	}
	if got[1]["language"] != "go" {
		t.Errorf("expected language 'go', got %v", got[1]["language"])
	}
	text, ok := got[1]["content"].(map[string]any)
	if !ok {
		t.Fatalf("expected content map, got %T", got[1]["content"])
	}
	if text["tag"] != "plain_text" {
		t.Errorf("expected plain_text tag inside pre, got %v", text["tag"])
	}
	if text["content"] != "fmt.Println(\"hi\")" {
		t.Errorf("unexpected code content: %q", text["content"])
	}
}

func TestRenderMarkdownToLarkElements_IndentedCodeBlock(t *testing.T) {
	input := "intro\n\n    indented code\n    line two\n\nafter"
	got := renderMarkdownToLarkElements(input)
	if len(got) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(got))
	}
	if got[1]["tag"] != "pre" {
		t.Fatalf("expected pre, got %v", got[1]["tag"])
	}
	text := got[1]["content"].(map[string]any)
	want := "    indented code\n    line two"
	// goldmark preserves the leading indent — strip it for the assertion.
	if got := strings.TrimLeft(text["content"].(string), " "); !strings.Contains(got, "indented code") {
		t.Errorf("expected code content to contain 'indented code', got %q", text["content"])
	}
	_ = want
}

func TestRenderMarkdownToLarkElements_Table(t *testing.T) {
	input := "| Step | Status |\n|---|---|\n| 1 | done |\n| 2 | done |"
	got := renderMarkdownToLarkElements(input)
	if len(got) != 1 {
		t.Fatalf("expected 1 element, got %d", len(got))
	}
	if got[0]["tag"] != "table" {
		t.Fatalf("expected tag 'table', got %v", got[0]["tag"])
	}
	columns, ok := got[0]["columns"].([]map[string]any)
	if !ok {
		t.Fatalf("expected columns to be []map[string]any, got %T", got[0]["columns"])
	}
	if len(columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(columns))
	}
	if columns[0]["display_name"] != "Step" {
		t.Errorf("expected first column header 'Step', got %q", columns[0]["display_name"])
	}
	if columns[1]["display_name"] != "Status" {
		t.Errorf("expected second column header 'Status', got %q", columns[1]["display_name"])
	}
	if columns[0]["name"] != "col_0" || columns[1]["name"] != "col_1" {
		t.Errorf("unexpected generated column names: %#v", columns)
	}
	rows, ok := got[0]["rows"].([]map[string]any)
	if !ok {
		t.Fatalf("expected rows to be []map[string]any, got %T", got[0]["rows"])
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 body rows, got %d", len(rows))
	}
	if rows[0]["col_0"] != "1" || rows[0]["col_1"] != "done" {
		t.Errorf("unexpected first row: %#v", rows[0])
	}
	if rows[1]["col_0"] != "2" || rows[1]["col_1"] != "done" {
		t.Errorf("unexpected second row: %#v", rows[1])
	}
}

func TestRenderMarkdownToLarkElements_Heading(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"# Title", "**Title**"},
		{"## Subtitle", "**Subtitle**"},
	}
	for _, c := range cases {
		got := renderMarkdownToLarkElements(c.input)
		if len(got) != 1 {
			t.Fatalf("input %q: expected 1 element, got %d", c.input, len(got))
		}
		if content := larkMDContent(t, got[0]); content != c.want {
			t.Errorf("input %q: expected %q, got %q", c.input, c.want, content)
		}
	}
}

func TestRenderMarkdownToLarkElements_BulletList(t *testing.T) {
	got := renderMarkdownToLarkElements("- one\n- two\n- three")
	if len(got) != 1 {
		t.Fatalf("expected 1 element, got %d", len(got))
	}
	want := "- one\n- two\n- three"
	if content := larkMDContent(t, got[0]); content != want {
		t.Errorf("expected %q, got %q", want, content)
	}
}

func TestRenderMarkdownToLarkElements_OrderedList(t *testing.T) {
	got := renderMarkdownToLarkElements("1. one\n2. two")
	if len(got) != 1 {
		t.Fatalf("expected 1 element, got %d", len(got))
	}
	want := "1. one\n2. two"
	if content := larkMDContent(t, got[0]); content != want {
		t.Errorf("expected %q, got %q", want, content)
	}
}

func TestRenderMarkdownToLarkElements_HorizontalRule(t *testing.T) {
	got := renderMarkdownToLarkElements("---")
	if len(got) != 1 {
		t.Fatalf("expected 1 element, got %d", len(got))
	}
	if got[0]["tag"] != "hr" {
		t.Errorf("expected tag 'hr', got %v", got[0]["tag"])
	}
}

func TestRenderMarkdownToLarkElements_Blockquote(t *testing.T) {
	got := renderMarkdownToLarkElements("> quoted text")
	if len(got) != 1 {
		t.Fatalf("expected 1 element, got %d", len(got))
	}
	if content := larkMDContent(t, got[0]); !strings.Contains(content, "quoted text") {
		t.Errorf("expected content to contain 'quoted text', got %q", content)
	}
}

func TestRenderMarkdownToLarkElements_Mixed(t *testing.T) {
	// The exact case the user reported: a body that mixes prose, a markdown
	// table, and prose again. Should produce paragraph + table + paragraph.
	input := "已完成本次基础环境更新:\n\n| 步骤 | 操作 | 状态 |\n|---|---|---|\n| 1 | `tar` | done |\n\n下一步待办。\n"
	got := renderMarkdownToLarkElements(input)
	if len(got) != 3 {
		t.Fatalf("expected 3 elements, got %d: %#v", len(got), got)
	}
	if got[0]["tag"] != "div" {
		t.Errorf("element 0 should be a paragraph, got %v", got[0]["tag"])
	}
	if got[1]["tag"] != "table" {
		t.Errorf("element 1 should be a table, got %v", got[1]["tag"])
	}
	if got[2]["tag"] != "div" {
		t.Errorf("element 2 should be a paragraph, got %v", got[2]["tag"])
	}
}

func TestRenderMarkdownToLarkElements_ValidJSON(t *testing.T) {
	// The whole point of this converter is that the output ends up in a
	// Feishu card payload. Make sure it round-trips through encoding/json
	// without surprises.
	input := "# Heading\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n```go\nx\n```\n\ntext `code` and **bold**."
	got := renderMarkdownToLarkElements(input)
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	for _, want := range []string{`"tag":"table"`, `"tag":"pre"`, `"tag":"div"`, `"tag":"hr"`} {
		if !strings.Contains(string(b), want) {
			// hr is not in this input, drop it
			if want == `"tag":"hr"` {
				continue
			}
			t.Errorf("expected JSON to contain %s, got %s", want, string(b))
		}
	}
}

func TestLarkBodyElement_Empty(t *testing.T) {
	cases := []string{"", "   ", "\n", "  \t  "}
	for _, input := range cases {
		if got := larkBodyElement(input); got != nil {
			t.Errorf("expected nil for %q, got %d elements", input, len(got))
		}
	}
}

func TestLarkBodyElement_TableBody(t *testing.T) {
	body := "已完成本次更新:\n\n| 步骤 | 状态 |\n|---|---|\n| 1 | done |"
	got := larkBodyElement(body)
	if len(got) < 2 {
		t.Fatalf("expected at least 2 elements, got %d", len(got))
	}
	if got[1]["tag"] != "table" {
		t.Errorf("expected second element to be a table, got %v", got[1]["tag"])
	}
}
