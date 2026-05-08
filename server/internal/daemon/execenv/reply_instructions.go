package execenv

import "fmt"

// BuildCommentReplyInstructions returns the canonical block telling an agent
// how to post its reply for a comment-triggered task. Both the per-turn
// prompt (daemon.buildCommentPrompt) and the CLAUDE.md workflow
// (InjectRuntimeConfig) call this so the trigger comment ID and the
// --parent value cannot drift between surfaces.
//
// The explicit "do not reuse --parent from previous turns" wording exists
// because resumed Claude sessions keep prior turns' tool calls in context
// and will otherwise copy the old --parent UUID forward.
//
// On Windows the template switches to `--content-file` because piping a
// HEREDOC through PowerShell 5.1 / cmd.exe re-encodes the bytes via the
// active console codepage, dropping non-ASCII characters as `?` before they
// reach `multica.exe` — see issues #2198 / #2236. Reading the body off disk
// preserves UTF-8 verbatim.
func BuildCommentReplyInstructions(issueID, triggerCommentID string) string {
	if triggerCommentID == "" {
		return ""
	}
	if runtimeGOOS == "windows" {
		return fmt.Sprintf(
			"If you decide to reply, post it as a comment — always use the trigger comment ID below, "+
				"do NOT reuse --parent values from previous turns in this session.\n\n"+
				"On Windows, write the reply body to a UTF-8 file with your file-write tool, then post it with `--content-file`. "+
				"Do NOT pipe via `--content-stdin` — Windows PowerShell 5.1 and cmd.exe re-encode piped bytes through the active console codepage and silently drop non-ASCII characters as `?`. "+
				"Do NOT use inline `--content`; it is easy to lose formatting or accidentally compress a structured reply into one line.\n\n"+
				"Use this form, preserving the same issue ID and --parent value:\n\n"+
				"    # 1. Write the reply body to a UTF-8 file (e.g. reply.md) with your file-write tool.\n"+
				"    # 2. Then run:\n"+
				"    multica issue comment add %s --parent %s --content-file ./reply.md\n\n"+
				"Do NOT write literal `\\n` escapes to simulate line breaks; the file preserves real newlines.\n",
			issueID, triggerCommentID,
		)
	}
	return fmt.Sprintf(
		"If you decide to reply, post it as a comment — always use the trigger comment ID below, "+
			"do NOT reuse --parent values from previous turns in this session.\n\n"+
			"Always use `--content-stdin` with a HEREDOC for agent-authored issue comments, even when the reply is a single line. "+
			"Do NOT use inline `--content`; it is easy to lose formatting or accidentally compress a structured reply into one line.\n\n"+
			"Use this form, preserving the same issue ID and --parent value:\n\n"+
			"    cat <<'COMMENT' | multica issue comment add %s --parent %s --content-stdin\n"+
			"    First paragraph.\n"+
			"\n"+
			"    Second paragraph.\n"+
			"    COMMENT\n\n"+
			"Do NOT write literal `\\n` escapes to simulate line breaks; the HEREDOC preserves real newlines.\n",
		issueID, triggerCommentID,
	)
}
