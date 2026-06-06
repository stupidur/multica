package handler

import (
	"encoding/json"
	"testing"
)

func TestLarkIssueURLTargetsReplyComposer(t *testing.T) {
	service := &LarkCardService{cfg: Config{PublicURL: "https://multica.example.com/"}}

	got := service.issueURL("acme", "issue-123")
	want := "https://multica.example.com/acme/issues/issue-123"
	if got != want {
		t.Fatalf("issueURL() = %q, want %q", got, want)
	}
}

func TestBuildInboxCardPutsReplyInputInFullWidthAction(t *testing.T) {
	service := &LarkCardService{}

	card := service.buildInboxCard(larkCardViewData{
		WorkspaceID: "workspace-1",
		IssueID:     "issue-1",
		IssueTitle:  "Issue title",
		IssueStatus: "in_review",
		ProjectName: "Project name",
		CommentBody: "Comment body",
		IssueURL:    "https://multica.example.com/acme/issues/issue-1",
	})

	elements, ok := card["elements"].([]map[string]any)
	if !ok || len(elements) < 3 {
		t.Fatalf("expected at least three card elements, got %#v", card["elements"])
	}
	inputActions, ok := elements[1]["actions"].([]map[string]any)
	if !ok || len(inputActions) != 1 {
		t.Fatalf("expected reply input to be isolated in its own action row, got %#v", elements[1]["actions"])
	}
	if inputActions[0]["tag"] != "input" {
		t.Fatalf("expected isolated action row to contain input, got %#v", inputActions[0])
	}
	if inputActions[0]["width"] != "fill" {
		t.Fatalf("expected reply input width=fill, got %#v", inputActions[0]["width"])
	}
	replyInputValue, ok := inputActions[0]["value"].(map[string]any)
	if !ok {
		t.Fatalf("expected reply input value, got %#v", inputActions[0]["value"])
	}
	if replyInputValue["action"] != "reply" || replyInputValue["workspace_id"] != "workspace-1" || replyInputValue["issue_id"] != "issue-1" || replyInputValue["body"] != "Comment body" {
		t.Fatalf("unexpected reply input value: %#v", replyInputValue)
	}
	buttonActions, ok := elements[2]["actions"].([]map[string]any)
	if !ok || len(buttonActions) != 2 {
		t.Fatalf("expected separate button action row, got %#v", elements[2]["actions"])
	}
	completeButtonValue, ok := buttonActions[0]["value"].(map[string]any)
	if !ok {
		t.Fatalf("expected complete button value, got %#v", buttonActions[0]["value"])
	}
	if completeButtonValue["action"] != "complete" || completeButtonValue["workspace_id"] != "workspace-1" || completeButtonValue["issue_id"] != "issue-1" {
		t.Fatalf("unexpected complete button value: %#v", completeButtonValue)
	}
	openIssueMultiURL, ok := buttonActions[1]["multi_url"].(map[string]any)
	if !ok {
		t.Fatalf("expected open issue multi_url, got %#v", buttonActions[1])
	}
	if openIssueMultiURL["url"] != "https://multica.example.com/acme/issues/issue-1" {
		t.Fatalf("unexpected open issue multi_url: %#v", openIssueMultiURL)
	}
}

func TestBuildInboxCardEscapesBodyAndUsesBlankFallback(t *testing.T) {
	service := &LarkCardService{}

	card := service.buildInboxCard(larkCardViewData{CommentBody: "Hello <world> & @Alice", IssueURL: "https://example.com"})
	bodyElement := card["elements"].([]map[string]any)[0]
	bodyText := bodyElement["text"].(map[string]any)
	if bodyText["content"] != "Hello &lt;world&gt; &amp; @Alice" {
		t.Fatalf("unexpected escaped body: %#v", bodyText["content"])
	}

	blankCard := service.buildInboxCard(larkCardViewData{CommentBody: "   ", IssueURL: "https://example.com"})
	blankElements := blankCard["elements"].([]map[string]any)
	if len(blankElements) != 2 {
		t.Fatalf("expected blank-body inbox card to omit body block, got %#v", blankElements)
	}
	if blankElements[0]["tag"] != "action" {
		t.Fatalf("expected first blank-body element to be action row, got %#v", blankElements[0])
	}
}

func TestBuildInboxCardUsesChineseCopyForChineseLanguage(t *testing.T) {
	service := &LarkCardService{}

	card := service.buildInboxCard(larkCardViewData{
		Language:    "zh-Hans",
		IssueURL:    "https://example.com",
		IssueID:     "issue-1",
		WorkspaceID: "workspace-1",
	})

	elements := card["elements"].([]map[string]any)
	if len(elements) != 2 {
		t.Fatalf("expected body-less inbox card to contain only action rows, got %#v", elements)
	}
	inputPlaceholder := elements[0]["actions"].([]map[string]any)[0]["placeholder"].(map[string]any)
	if inputPlaceholder["content"] != "回复 Multica..." {
		t.Fatalf("unexpected Chinese placeholder: %#v", inputPlaceholder["content"])
	}
	buttons := elements[1]["actions"].([]map[string]any)
	if buttons[0]["text"].(map[string]any)["content"] != "标记完成" {
		t.Fatalf("unexpected Chinese done label: %#v", buttons[0]["text"])
	}
	if buttons[1]["text"].(map[string]any)["content"] != "打开问题" {
		t.Fatalf("unexpected Chinese open label: %#v", buttons[1]["text"])
	}

	completed := service.buildCompletedCard(larkCardViewData{Language: "zh-Hans", IssueURL: "https://example.com"})
	completedElements := completed["elements"].([]map[string]any)
	if len(completedElements) != 2 {
		t.Fatalf("expected body-less completed card to contain note and action, got %#v", completedElements)
	}
	if completedElements[0]["elements"].([]map[string]any)[0]["content"] != "已在飞书标记完成，并同步到 Multica。" {
		t.Fatalf("unexpected Chinese completion note: %#v", completedElements[0]["elements"])
	}
	if completedElements[1]["actions"].([]map[string]any)[0]["text"].(map[string]any)["content"] != "打开问题" {
		t.Fatalf("unexpected Chinese completed open label: %#v", completedElements[1]["actions"])
	}
}

func TestBuildRepliedCardShowsRepliedState(t *testing.T) {
	service := &LarkCardService{}

	card := service.buildRepliedCard(larkCardViewData{
		Language:    "zh-Hans",
		CommentBody: "Original body",
		IssueURL:    "https://multica.example.com/acme/issues/issue-1",
	})

	elements := card["elements"].([]map[string]any)
	if len(elements) != 3 {
		t.Fatalf("expected body, replied note, and open issue action, got %#v", elements)
	}
	noteElements := elements[1]["elements"].([]map[string]any)
	if noteElements[0]["content"] != "已回复，并同步到 Multica。" {
		t.Fatalf("unexpected replied note: %#v", noteElements)
	}
	openIssueActions := elements[2]["actions"].([]map[string]any)
	if len(openIssueActions) != 1 {
		t.Fatalf("expected only open issue action after reply, got %#v", openIssueActions)
	}
	if openIssueActions[0]["text"].(map[string]any)["content"] != "打开问题" {
		t.Fatalf("unexpected open issue label: %#v", openIssueActions[0]["text"])
	}
}

func TestBuildRepliedCardOmitsBlankBody(t *testing.T) {
	service := &LarkCardService{}

	card := service.buildRepliedCard(larkCardViewData{
		CommentBody: "   ",
		IssueURL:    "https://multica.example.com/acme/issues/issue-1",
	})

	elements := card["elements"].([]map[string]any)
	if len(elements) != 2 {
		t.Fatalf("expected blank-body replied card to omit body block, got %#v", elements)
	}
	if elements[0]["tag"] != "note" {
		t.Fatalf("expected first blank-body replied element to be note, got %#v", elements[0])
	}
}

func TestBuildCompletedCardOmitsBlankBody(t *testing.T) {
	service := &LarkCardService{}

	card := service.buildCompletedCard(larkCardViewData{
		CommentBody: "   ",
		IssueURL:    "https://multica.example.com/acme/issues/issue-1",
	})

	elements := card["elements"].([]map[string]any)
	if len(elements) != 2 {
		t.Fatalf("expected blank-body completed card to omit body block, got %#v", elements)
	}
	if elements[0]["tag"] != "note" {
		t.Fatalf("expected first blank-body completed element to be note, got %#v", elements[0])
	}
}

// TestBuildInboxCardRendersMarkdownTable is a demonstration / smoke test:
// given a body that matches the markdown the agent emits (lead-in prose,
// GFM table, trailing prose, inline code, URL), it builds the full Feishu
// card and prints the resulting payload so a human reviewer can eyeball it.
//
// Run with:
//
//	go test ./internal/handler/ -run TestBuildInboxCardRendersMarkdownTable -v
//
// and the full card JSON will be logged under the test output.
func TestBuildInboxCardRendersMarkdownTable(t *testing.T) {
	body := "已完成本次基础环境、本地服务、multica 和 Dify 更新；未执行备份回滚，因为最终验证通过。\n\n" +
		"| 步骤 | 操作 / 命令 | 状态 | 耗时 |\n" +
		"|---|---|---|---|\n" +
		"| 步骤1 | `tar -czf /home/stupidur/.openclaw.multica-latest-backup.tgz` | 成功 | 约20分钟 |\n" +
		"| 步骤2 | `npm install -g openclaw@latest` | 成功: OpenClaw `2026.6.1`; gateway `18789` active | 约12分钟 |\n" +
		"| 步骤3 | `hermes update` | 成功: Hermes Agent `v0.16.0`; dashboard `9119` 可访问 | 约18分钟 |\n" +
		"\n" +
		"验证 frontend `http://127.0.0.1:3000/` 正常返回 `/login` 重定向。\n"

	service := &LarkCardService{}
	card := service.buildInboxCard(larkCardViewData{
		WorkspaceID: "acme",
		IssueID:     "issue-123",
		IssueTitle:  "服务器基础环境",
		IssueStatus: "in_progress",
		ProjectName: "服务器基础环境",
		CommentBody: body,
		IssueURL:    "https://multica.example.com/acme/issues/issue-123",
		Language:    "zh-Hans",
	})

	payload, err := json.MarshalIndent(card, "", "  ")
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	t.Logf("\n--- full Feishu card payload ---\n%s\n--- end ---", string(payload))

	elements := card["elements"].([]map[string]any)
	if elements[0]["tag"] != "div" {
		t.Fatalf("expected first element to be a div, got %v", elements[0]["tag"])
	}

	var table map[string]any
	for _, el := range elements {
		if el["tag"] == "table" {
			table = el
			break
		}
	}
	if table == nil {
		t.Fatal("expected at least one table element in the rendered card body")
	}
	columns := table["columns"].([]map[string]any)
	if len(columns) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(columns))
	}
	if columns[0]["display_name"] != "步骤" || columns[1]["display_name"] != "操作 / 命令" {
		t.Errorf("unexpected table headers: %#v", columns)
	}
	rows := table["rows"].([]map[string]any)
	if len(rows) != 3 {
		t.Errorf("expected 3 body rows, got %d", len(rows))
	}
	if rows[0]["col_0"] != "步骤1" || rows[0]["col_3"] != "约20分钟" {
		t.Errorf("unexpected first data row: %#v", rows[0])
	}
}
