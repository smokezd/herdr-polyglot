# Changelog

All notable changes to this project are written down here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
the versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- The reader keeps the last 100 translated replies of each project — the
  repository the agent works in, each worktree on its own. A reply translated
  before is shown at once without asking the service again, and `[` and `]` step
  back and forth through the earlier ones. A new reply waits for `t`: the popup
  opens on the latest translation kept, and the service is asked nothing until
  then.
- The reader can render what it shows as markdown, the way glow does: `M`
  switches it in the reader and in a reply previewed whole,
  `HERDR_POLYGLOT_READ_MARKDOWN` turns it on from the start, and
  `HERDR_POLYGLOT_READ_MARKDOWN_STYLE` picks one of glamour's styles.
- In the reader, `l` lists every reply of the *Claude Code* or *Codex* session
  under the prompt it answered, ✓ on those translated already, and `enter`
  translates the one picked.
- LM Studio can run on more than one machine: `HERDR_POLYGLOT_LMSTUDIO_HOSTS`
  names them as `name=url,…`, each with its own model and key if it needs one, and
  `ctrl+o` in any popup lists them, says which are up, and switches. The choice is
  remembered for the popups after it. The header names the host in use, as
  "LM Studio on macstudio".
- In the reader, `y` copies all of what is on show to the clipboard, the
  translation or the original, however far it is scrolled; and `ctrl+b` pages up,
  as `space` pages down.
- The agent's replies can be read in your own language: the action
  `wazum.polyglot.read` translates the latest reply into
  `HERDR_POLYGLOT_READ_LANGUAGE` and shows it in a popup. `HERDR_POLYGLOT_READ_REPLIES`
  shows more than one, and `HERDR_POLYGLOT_READ_CONTEXT` (2 by default) sends that
  many replies before them along as context. *Claude Code* and *Codex* replies come
  from their session files; any other agent's from the terminal.
  With *LM Studio* the translation streams in as it is written, and a model's
  thinking is shown while it goes on and folded into one line once the answer
  begins — whether the server sends it in `reasoning_content`, `reasoning` or
  `<think>` tags.
- The agent's replies can be read in a pane beside it that leaves the keyboard
  where it was: `wazum.polyglot.read-pane`, which translates again in the pane
  already open. `m`, or `wazum.polyglot.read-toggle`, moves what is on show
  between popup and pane.
- A *Claude Code* reply that asks you something now has the question and its
  numbered options in it, and one that asks you to approve a plan has the plan.
- Both popups can be sized: `HERDR_POLYGLOT_WIDTH` and `HERDR_POLYGLOT_HEIGHT` for
  the prompt, `HERDR_POLYGLOT_READ_WIDTH` and `HERDR_POLYGLOT_READ_HEIGHT` for the
  replies, each in cells or as a share of the screen such as `80%`. The text fills
  whatever width it is given.

- A language model on your own machine translates, through
  [LM Studio](https://lmstudio.ai)'s local server or anything else that answers
  OpenAI-style chat completions: `HERDR_POLYGLOT_PROVIDER=lmstudio`, and it is the
  default. `HERDR_POLYGLOT_MODEL` picks a model, the loaded one otherwise;
  `HERDR_POLYGLOT_SYSTEM_PROMPT_EXTRA`, `HERDR_POLYGLOT_USER_PROMPT_EXTRA` and
  `HERDR_POLYGLOT_REQUEST_EXTRA` add to the instructions, to the draft's message and
  to the request. See [docs/local-translation.md](docs/local-translation.md#lm-studio).

- A translation service can be a program on your own machine:
  `HERDR_POLYGLOT_COMMAND` names a command line, the draft is written to its input
  and its answer is the translation. No key, no account, nothing over the network —
  [translateLocally](https://translatelocally.com) is quick enough for live
  translation. See [docs/local-translation.md](docs/local-translation.md).
- `HERDR_POLYGLOT_PROVIDER=off` opens the popup as a prompt box that hands what
  you write to the agent untranslated, with every key, the vim bindings and the
  resumed draft.
- `tab` flips between writing and reading. Reading gives the translation the whole
  popup, scrolled with `↑ ↓ PageUp PageDown` — or `j k g G` with vim bindings on —
  and shows how far down it you are. Any letter, `tab` or `esc` goes back to the
  draft, so it cannot be got stuck in.
- `ctrl+t` translates the draft as it stands: the way to try again when a
  translation did not arrive, and the way to read the English at all with live
  translation off, without sending anything.
- The panel being written in or read is drawn with the accent border, the other in
  the frame's grey, so it is clear which one has the keys.
- How far through a panel you are is written into its bottom border rather than
  the footer.

### Changed

- A draft leaves the machine only for a service that is named. With nothing
  configured the model in LM Studio translates, where the popup used to be a plain
  prompt box — `HERDR_POLYGLOT_PROVIDER=off` asks for that box. A key on its own no
  longer chooses *DeepL*: set `HERDR_POLYGLOT_PROVIDER=deepl` (or `google`) beside it.
- A missing or unusable key is said in the popup, naming the file it belongs in,
  instead of the plugin refusing to open.

### Fixed

- A reader opened while *Claude Code* is still working on your latest prompt
  waits for its answer, saying so, instead of showing the reply before it as if it
  were the answer. A slash command such as `/copy` is not taken for a prompt to
  wait on.
- An error *Claude Code* writes in the agent's place, such as "Your computer went
  to sleep mid-response", is no longer read as part of the reply.
- A *Claude Code* session that went on under a new id is read where it went on.
  Herdr can keep reporting the old id, whose file stops at the move, so the
  reader showed a reply from before it.
- A service that times out or cannot be reached says so in a sentence, instead of
  showing a Go error with a URL and a client timeout in it.
- Reading a translation that failed shows what went wrong, rather than claiming
  nothing has been translated yet.
- The boxes are given their rows again when a translation arrives or a
  confirmation opens, so the popup does not outgrow its pane until live
  translation is toggled.
- The scrollbar says where the view actually is. It was worked out from the cursor
  on the assumption that the last row was on screen, so it claimed the top while
  rows were hidden above.
- A draft that comes back opens at its beginning, where it can be read, instead of
  scrolled to its end.
- The draft can be walked through when it holds more than it shows: the arrows
  move by row, and `gj`/`gk` do the same with vim bindings on, where `j`/`k` keep
  vim's meaning of a whole line.
- Pasting a long draft no longer leaves the view at the top with the cursor out of
  sight below.
- Nothing is drawn wider than the pane. A popup narrower than 34 columns used to
  be drawn at 34 and wrap every line, stacking frame on frame while scrolling.
- The translation beside the draft shows its beginning and ends in `…` when there
  is more, instead of a scrollbar that could not be used from there. `tab` reads
  the rest, and the footer says so while there is more to read.
- Text no longer wraps twice. The draft box was two columns wider than what it
  showed, so a line was wrapped once by the text area and again by the box, which
  dropped words onto lines of their own.
- The popup keeps its shape whatever is written in it. A long translation used to
  grow its box and push the footer out of the pane; both boxes now scroll, with a
  bar showing how much is out of view, and the keys drop off the footer one at a
  time when a pane is too narrow for them.

## [0.3.0] - 2026-08-18

### Added

- The plugin signs the empty draft box with a small braille mark, in the corner
  furthest from the writing. It goes as soon as there is anything in the box, and
  `HERDR_POLYGLOT_LOGO=0` leaves the box bare.

### Changed

- A delivered prompt keeps its line breaks instead of being flattened onto one
  line, so a protected code block arrives as a code block and a `//` comment no
  longer swallows the rest of it. Tabs arrive as four spaces; escape sequences are
  still stripped.
- Code is no longer translated. Backticked spans and fenced blocks are taken out
  of the draft before it is sent and put back exactly as they were, so identifiers
  keep their names, comments and string literals stay as written, and indentation
  survives. The code never reaches the service, so it costs nothing to translate
  and stays on your machine.

## [0.2.0] - 2026-08-18

### Added

- *Google Cloud Translation* as a second service, chosen with
  `HERDR_POLYGLOT_PROVIDER=google` and a key in `HERDR_POLYGLOT_GOOGLE_API_KEY`.
  The language setting is shared: `EN-US` reaches *DeepL* as it stands and
  *Google* as `en`, and a region is kept only where the region is the point.

## [0.1.0] - 2026-08-18

First release. Settings, keys and behaviour may still change while the plugin
meets other people's terminals — that is what the 0.x is for.

### Added

- An overlay over any agent pane, opened by a keybinding. Write the prompt in
  your own language; `alt+enter` translates it and hands it over.
- Two ways to deliver it: sent to the agent, or typed into its input for you to
  send. `ctrl+r` switches between them while you write, and the header says which
  it will be.
- *DeepL* as the translation service, with `dry-run` for checking the wiring
  without a key. The service sits behind an interface, so another one is a
  package away.
- Live translation, off by default: the English follows the draft as you write.
  Each sentence is paid for once, and a translation you have already read is
  delivered without being translated again. It stays off for a draft that came
  back from an earlier session and for pasted text until `ctrl+l` asks for it.
- Confirmation mode: see the English and press `alt+enter` again to deliver it.
- Unfinished drafts kept privately, one per pane, and restored the next time the
  popup opens — however it was closed, including *herdr* closing it.
- Vim bindings in the draft box, off by default: the motions, edits, counts and
  registers that make sense inside a text box.
- The month's character count in the header, when the service reports it, and a
  warning when a draft grows past prompt size.
- Settings from the plugin's `.env` or the environment. A value that is neither
  on nor off is refused rather than guessed at.
- Prebuilt binaries for macOS and Linux on arm64 and amd64, published with
  checksums and build provenance. Installing needs no Go toolchain.

[Unreleased]: https://github.com/wazum/herdr-polyglot/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/wazum/herdr-polyglot/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/wazum/herdr-polyglot/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/wazum/herdr-polyglot/releases/tag/v0.1.0
