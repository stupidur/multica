package handler

import "testing"

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
	blankBodyText := blankCard["elements"].([]map[string]any)[0]["text"].(map[string]any)
	if blankBodyText["content"] != "No comment content" {
		t.Fatalf("unexpected blank body fallback: %#v", blankBodyText["content"])
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
	inputPlaceholder := elements[1]["actions"].([]map[string]any)[0]["placeholder"].(map[string]any)
	if inputPlaceholder["content"] != "回复 Multica..." {
		t.Fatalf("unexpected Chinese placeholder: %#v", inputPlaceholder["content"])
	}
	buttons := elements[2]["actions"].([]map[string]any)
	if buttons[0]["text"].(map[string]any)["content"] != "标记完成" {
		t.Fatalf("unexpected Chinese done label: %#v", buttons[0]["text"])
	}
	if buttons[1]["text"].(map[string]any)["content"] != "打开问题" {
		t.Fatalf("unexpected Chinese open label: %#v", buttons[1]["text"])
	}

	completed := service.buildCompletedCard(larkCardViewData{Language: "zh-Hans", IssueURL: "https://example.com"})
	completedElements := completed["elements"].([]map[string]any)
	if completedElements[1]["elements"].([]map[string]any)[0]["content"] != "已在飞书标记完成，并同步到 Multica。" {
		t.Fatalf("unexpected Chinese completion note: %#v", completedElements[1]["elements"])
	}
	if completedElements[2]["actions"].([]map[string]any)[0]["text"].(map[string]any)["content"] != "打开问题" {
		t.Fatalf("unexpected Chinese completed open label: %#v", completedElements[2]["actions"])
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
