---
name: Audio Transcription
description: Transcribe audio files using local Whisper CLI (fallback when API is unavailable)
version: 1.0.0
metadata:
  emoji: "🎙️"
  requires:
    anyBins:
      - whisper
      - whisper-cli
  install:
    - id: brew-whisper
      kind: brew
      formula: openai-whisper
      bins: [whisper]
      label: "Install OpenAI Whisper via Homebrew"
      os: [darwin]
  tags:
    - audio
    - transcription
    - media
userInvocable: false
disableModelInvocation: false
---

## Audio Transcription (Local Fallback)

Voice messages from channels are normally transcribed automatically via the OpenAI Whisper API before reaching you. This skill is only needed when the API is unavailable.

If you receive `[audio message received]` with a `File:` path (instead of `[Voice Message]` with a transcript), it means the API transcription was not available. Use local whisper to transcribe:

```
whisper "<file_path>" --model base --output_format txt --output_dir /tmp
```

Then read the `.txt` file from `/tmp/` and respond based on the transcribed content.
