package execenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Pins runtimeGOOS to "linux": the Windows reply template has its own
// dedicated test (TestBuildCommentReplyInstructionsWindowsUsesContentFile).
// Not parallel: mutates the package-level runtimeGOOS.
func TestBuildCommentReplyInstructionsIncludesTriggerID(t *testing.T) {
	saved := runtimeGOOS
	t.Cleanup(func() { runtimeGOOS = saved })
	runtimeGOOS = "linux"

	issueID := "11111111-1111-1111-1111-111111111111"
	triggerID := "22222222-2222-2222-2222-222222222222"

	got := BuildCommentReplyInstructions(issueID, triggerID)

	for _, want := range []string{
		"multica issue comment add " + issueID + " --parent " + triggerID,
		"Always use `--content-stdin`",
		"even when the reply is a single line",
		"--content-stdin",
		"<<'COMMENT'",
		"Do NOT write literal `\\n` escapes to simulate line breaks",
		"do NOT reuse --parent values from previous turns",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("reply instructions missing %q\n---\n%s", want, got)
		}
	}

	if strings.Contains(got, "--content \"...\"") {
		t.Fatalf("reply instructions should not offer inline --content form\n---\n%s", got)
	}
}

func TestBuildCommentReplyInstructionsEmptyWhenNoTrigger(t *testing.T) {
	t.Parallel()

	if got := BuildCommentReplyInstructions("issue-id", ""); got != "" {
		t.Fatalf("expected empty string when triggerCommentID is empty, got %q", got)
	}
}

// Pins runtimeGOOS to "linux" so the helper output is deterministic.
// Not parallel: mutates the package-level runtimeGOOS.
func TestInjectRuntimeConfigCommentTriggerUsesHelper(t *testing.T) {
	saved := runtimeGOOS
	t.Cleanup(func() { runtimeGOOS = saved })
	runtimeGOOS = "linux"

	dir := t.TempDir()

	issueID := "11111111-1111-1111-1111-111111111111"
	triggerID := "22222222-2222-2222-2222-222222222222"

	ctx := TaskContextForEnv{
		IssueID:          issueID,
		TriggerCommentID: triggerID,
	}
	if err := InjectRuntimeConfig(dir, "claude", ctx); err != nil {
		t.Fatalf("InjectRuntimeConfig failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}

	s := string(content)
	for _, want := range []string{
		triggerID,
		"multica issue comment add " + issueID + " --parent " + triggerID,
		"do NOT reuse --parent values from previous turns",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("CLAUDE.md missing %q", want)
		}
	}
}

// TestBuildCommentReplyInstructionsWindowsUsesContentFile pins that on Windows
// hosts every per-turn reply prompt — and the workflow block in CLAUDE.md /
// AGENTS.md — points agents at `--content-file` instead of `--content-stdin`.
// Without this, the Windows console codepage re-encodes piped HEREDOC bytes
// and silently drops non-ASCII characters as `?` before they reach
// `multica.exe` (issues #2198, #2236).
//
// Not parallel: mutates the package-level `runtimeGOOS`. Restores it via
// t.Cleanup so any subsequent `t.Parallel()` tests see the original value.
func TestBuildCommentReplyInstructionsWindowsUsesContentFile(t *testing.T) {
	saved := runtimeGOOS
	t.Cleanup(func() { runtimeGOOS = saved })

	issueID := "11111111-1111-1111-1111-111111111111"
	triggerID := "22222222-2222-2222-2222-222222222222"

	t.Run("windows host points at --content-file", func(t *testing.T) {
		runtimeGOOS = "windows"
		got := BuildCommentReplyInstructions(issueID, triggerID)
		for _, want := range []string{
			"multica issue comment add " + issueID + " --parent " + triggerID + " --content-file",
			"--content-file",
			"On Windows, write the reply body to a UTF-8 file",
			"Do NOT pipe via `--content-stdin`",
			"silently drop non-ASCII characters as `?`",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("Windows reply instructions missing %q\n---\n%s", want, got)
			}
		}
		for _, banned := range []string{
			"<<'COMMENT'",
			"--content-stdin\n",
			"cat <<",
		} {
			if strings.Contains(got, banned) {
				t.Errorf("Windows reply instructions should not contain %q\n---\n%s", banned, got)
			}
		}
	})

	t.Run("non-windows host keeps --content-stdin HEREDOC", func(t *testing.T) {
		runtimeGOOS = "linux"
		got := BuildCommentReplyInstructions(issueID, triggerID)
		for _, want := range []string{
			"multica issue comment add " + issueID + " --parent " + triggerID + " --content-stdin",
			"<<'COMMENT'",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("Linux reply instructions missing %q\n---\n%s", want, got)
			}
		}
		if strings.Contains(got, "--content-file") {
			t.Errorf("Linux reply instructions should not push --content-file\n---\n%s", got)
		}
	})
}

// TestInjectRuntimeConfigWindowsCommentTriggerHasNoStdin asserts the
// end-to-end CLAUDE.md / AGENTS.md surface for a comment-triggered task on a
// Windows daemon: the Available Commands section, the Codex-specific
// paragraph, AND the per-turn reply template all line up on `--content-file`,
// with no remaining `--content-stdin` directive that would override the
// Windows fallback. Pins the bug GPT-Boy flagged on PR #2247: the original
// fix only patched Available Commands, leaving the Codex section and per-turn
// prompt still mandating stdin.
func TestInjectRuntimeConfigWindowsCommentTriggerHasNoStdin(t *testing.T) {
	saved := runtimeGOOS
	t.Cleanup(func() { runtimeGOOS = saved })
	runtimeGOOS = "windows"

	issueID := "11111111-1111-1111-1111-111111111111"
	triggerID := "22222222-2222-2222-2222-222222222222"
	ctx := TaskContextForEnv{
		IssueID:          issueID,
		TriggerCommentID: triggerID,
	}

	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			if err := InjectRuntimeConfig(dir, provider, ctx); err != nil {
				t.Fatalf("InjectRuntimeConfig failed: %v", err)
			}
			fileName := "CLAUDE.md"
			if provider != "claude" {
				fileName = "AGENTS.md"
			}
			data, err := os.ReadFile(filepath.Join(dir, fileName))
			if err != nil {
				t.Fatalf("read %s: %v", fileName, err)
			}
			s := string(data)

			for _, want := range []string{
				"multica issue comment add " + issueID + " --parent " + triggerID + " --content-file",
				"--content-file",
				"--description-file",
				"On this Windows host",
			} {
				if !strings.Contains(s, want) {
					t.Errorf("%s missing %q\n---\n%s", fileName, want, s)
				}
			}

			// The Available Commands section, the Codex paragraph, and the
			// per-turn reply template must NOT end up prescriptively
			// directing the agent at stdin — that is the exact pattern
			// Windows shells mangle. Covers GPT-Boy's second blocker on
			// PR #2247: the original Windows-only fallback was appended
			// *after* unconditional "MUST pipe via stdin" /
			// `--description-stdin` lines, leaving agents with
			// conflicting instructions. We pin the prescriptive
			// phrasings (not bare flag names) so anti-prescriptive prose
			// like "do NOT pipe via `--content-stdin`" doesn't trip the
			// ban.
			for _, banned := range []string{
				"--parent " + triggerID + " --content-stdin",
				"always use `--content-stdin` with a HEREDOC, even for short single-line replies",
				"MUST pipe via stdin",
				"use `--description-stdin` and pipe a HEREDOC",
				"<<'COMMENT'",
				"Agent-authored comments should always pipe content via stdin",
			} {
				if strings.Contains(s, banned) {
					t.Errorf("%s still steers agent at stdin: %q\n---\n%s", fileName, banned, s)
				}
			}
		})
	}
}
