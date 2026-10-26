# polyglot

[![CI](https://github.com/wazum/herdr-polyglot/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/wazum/herdr-polyglot/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/wazum/herdr-polyglot?color=6C3EF5)](https://github.com/wazum/herdr-polyglot/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/wazum/herdr-polyglot)](go.mod)
[![herdr](https://img.shields.io/badge/herdr-%E2%89%A5%200.8.0-6C3EF5)](https://herdr.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Write prompts in the language you think in and keep the conversation in English.
A [*herdr*](https://herdr.dev) plugin: press a key on any agent pane, an overlay
opens above it, you write, and the English translation lands in the agent's
input. It works with *Claude Code*, *Codex*, *opencode* and the rest, because
the prompt goes through *herdr* rather than into a particular tool.

![the popup over a Claude Code pane: a German prompt written, translated, and
typed into the agent's input](demo/polyglot-in-herdr.gif?1787075576)

## Why this exists

You think faster in your own language. But an agent answers in the language it was
asked in, so your prompts end up in the replies, the comments, the commits and the
docs — and a rule in `CLAUDE.md` or `AGENTS.md` cannot hold: the agent continues the
context it has, and every prompt you send makes more of that context your language.
Translate the prompt and the drift has no source.

Side effect: your sentence and its English sit side by side, prompt after prompt.
It rubs off.

## Quick start

```bash
herdr plugin install wazum/herdr-polyglot
```

That is enough to use it: with nothing configured the draft is translated by the
model loaded in [LM Studio](https://lmstudio.ai) on this machine, and nothing
leaves it. Start LM Studio's server with a model loaded —
[how to set that up](docs/local-translation.md#lm-studio). Without it the footer
says LM Studio could not be reached, and `HERDR_POLYGLOT_PROVIDER=off` makes the
popup a plain prompt box instead.

To translate through *DeepL*, name it and put your key in the plugin's own config
directory, readable only by you — a plain redirect would leave it readable by
everyone on the machine:

```bash
ENV_FILE="$(herdr plugin config-dir wazum.polyglot)/.env"
touch "$ENV_FILE" && chmod 600 "$ENV_FILE"
echo "HERDR_POLYGLOT_PROVIDER=deepl" >> "$ENV_FILE"
echo "HERDR_POLYGLOT_API_KEY=your-deepl-key" >> "$ENV_FILE"
```

A [*DeepL* API key](https://www.deepl.com/pro-api) has a free tier. Free keys
end in `:fx`, and the plugin sends those to *DeepL*'s free host by itself.

Then bind a key in `~/.config/herdr/config.toml`:

```toml
[[keys.command]]
key = "prefix+t"
type = "plugin_action"
command = "wazum.polyglot.prompt"
description = "write a prompt in your own language"
```

For one gesture instead of two, let the terminal send the prefix for you. In
*Ghostty*:

```
keybind = super+p=text:\x02t
```

`\x02` is `ctrl+b`, the prefix, and `t` is the binding above. Reload with ⌘⇧, and
take a chord the terminal has free — ⌘G is find-next in *Ghostty*.

`t` as in translate — *herdr* already uses `prefix+p` for the previous tab.
There is a second action, `wazum.polyglot.compose`, which types the prompt into
the agent's input instead of sending it, and `ctrl+r` switches between the two
while you write, so one keybinding is enough.

## In the popup

| | |
| --- | --- |
| `alt+enter` | translate and hand the prompt over (`ctrl+d` does the same) |
| `enter` | a new line, because a prompt is often more than one |
| `ctrl+r` | switch between sending it and only filling the input |
| `ctrl+l` | turn live translation on or off for this prompt |
| `ctrl+t` | translate what is there now, or try again after a translation failed |
| `tab` | read the translation across the whole popup, and back |
| `ctrl+o` | pick which [LM Studio machine](docs/local-translation.md#more-than-one-machine) translates |
| `ctrl+u` | throw the draft away, as it clears a line in a shell |
| `esc` | close — with vim bindings on, first to normal mode, then close |
| `ctrl+c` | close, always |

The header says which of the two will happen: `sends to agent` hands the prompt
over and the agent starts working, `fills the input` types it there and leaves
the last keystroke to you. When something does not work the footer says so,
`esc` takes the message away, and it goes by itself after a few seconds.

## Settings

Every setting can be a line in the `.env` file or an environment variable, which
wins over the file. A value that is neither on (`1`, `true`, `yes`, `on`) nor
off (`0`, `false`, `no`, `off`) is refused rather than guessed at.

| Setting | Default | Meaning |
| --- | --- | --- |
| `HERDR_POLYGLOT_API_KEY` | none | Credentials for the translation service |
| `HERDR_POLYGLOT_PROVIDER` | a local one | Which service: `lmstudio`, `deepl`, `google`, `cmd`, `dry-run` or `off` |
| `HERDR_POLYGLOT_COMMAND` | none | A program that translates, for [local translation](docs/local-translation.md) |
| `HERDR_POLYGLOT_LANGUAGE` | `EN-US` | Target language |
| `HERDR_POLYGLOT_ENDPOINT` | the service's own | Override the service endpoint |
| `HERDR_POLYGLOT_MODEL` | the loaded one | Which model `lmstudio` asks for |
| `HERDR_POLYGLOT_LMSTUDIO_HOSTS` | none | [Several machines](docs/local-translation.md#more-than-one-machine) running LM Studio, as `name=url,…` |
| `HERDR_POLYGLOT_SYSTEM_PROMPT_EXTRA` | none | Added to the instructions `lmstudio` gives the model |
| `HERDR_POLYGLOT_USER_PROMPT_EXTRA` | none | Added after the draft for `lmstudio`, e.g. `/no_think` |
| `HERDR_POLYGLOT_REQUEST_EXTRA` | none | A JSON object merged into the `lmstudio` request |
| `HERDR_POLYGLOT_SUBMIT` | `1` | `0` types the prompt without sending it |
| `HERDR_POLYGLOT_VIM` | `0` | `1` turns on the [vim bindings](docs/vim.md) |
| `HERDR_POLYGLOT_LIVE` | `0` | `1` translates while you write |
| `HERDR_POLYGLOT_CONFIRM` | `0` | `1` shows the English and waits for a second `alt+enter` |
| `HERDR_POLYGLOT_KEEP_DRAFT` | `1` | `0` starts from an empty box instead of resuming |
| `HERDR_POLYGLOT_MAX_DRAFT` | `2000` | Characters before the box says the draft is too long |
| `HERDR_POLYGLOT_PULSE` | `1` | `0` stops the live circle from breathing |
| `HERDR_POLYGLOT_LOGO` | `1` | `0` leaves the empty draft box unsigned |
| `HERDR_POLYGLOT_READ_LANGUAGE` | none | Language the agent's replies are [read in](#read-the-agents-reply-in-your-language) |
| `HERDR_POLYGLOT_READ_REPLIES` | `1` | How many of the latest replies are translated and shown |
| `HERDR_POLYGLOT_READ_CONTEXT` | `2` | How many replies before those go along as context, unshown |
| `HERDR_POLYGLOT_READ_MARKDOWN` | `1` | `0` shows what the reader shows as plain text rather than rendered as markdown, as glow does; `M` switches it |
| `HERDR_POLYGLOT_READ_MARKDOWN_STYLE` | `dark` | Its look: one of glamour's styles, `dark`, `light`, `dracula`, `tokyo-night`, `pink`, `ascii` or `notty`; `S` goes through them, and the one it chose last wins |
| `HERDR_POLYGLOT_WIDTH` | `90` | Width of the prompt popup, in cells or as a share of the screen like `80%` |
| `HERDR_POLYGLOT_HEIGHT` | `17` | Height of the prompt popup, the same way |
| `HERDR_POLYGLOT_READ_WIDTH` | `90` | Width of the popup replies are read in |
| `HERDR_POLYGLOT_READ_HEIGHT` | `32` | Height of the popup replies are read in |

A draft leaves the machine only for a service you named. With no
`HERDR_POLYGLOT_PROVIDER`, a command means that command and nothing at all means
`lmstudio`, the model in LM Studio. A key is not enough on its own — it may be left
over — so a key without a named service is refused, and so is a key together with a
command; `HERDR_POLYGLOT_PROVIDER` then decides. The four model settings apply to
`lmstudio` only, and are refused with any other service that translates. Asking for a
service that cannot be built is a case of its own: the popup says what is missing
and where it belongs, and delivers nothing, so an untranslated draft never reaches
the agent by accident. To keep keys for several services side by side, scope them by
name: `HERDR_POLYGLOT_DEEPL_API_KEY`, `HERDR_POLYGLOT_GOOGLE_API_KEY`.

*Google* takes the same language setting and needs the Cloud Translation API
enabled for the key. It reports no monthly count, so the header shows none with
it, and it has no unbilled context parameter — live translation still pays for
each sentence once, but translates each on its own.

## What else it does

**An unfinished prompt is kept.** Drafts are stored privately, one per pane, and
come back the next time you open the popup there, however it closed. A sent or
discarded draft is forgotten.

**Read the English first.** With `HERDR_POLYGLOT_CONFIRM=1`, `alt+enter`
translates and shows the result, a second `alt+enter` delivers it, and `esc`
goes back to writing. It costs one translation, not two.

**Live translation.** The English follows your draft after a short pause. Each
sentence is paid for once, and a translation you have already read is delivered
as it stands, so writing costs little more than sending —
[how that works](docs/live-translation.md). Live translation starts off for a
draft that came back from an earlier session and for text you paste in, since
neither is something you asked to have translated; `ctrl+l` turns it on.

**Translation on your own machine.** By default a model in LM Studio translates,
and `HERDR_POLYGLOT_COMMAND` points the plugin at any other program instead, so a
draft need not leave the machine at all — [how to set that up](docs/local-translation.md).

**Code is not translated.** A translation service rewrites code as if it were
prose: it renames identifiers, translates comments and string literals, and
reformats indentation. So anything in backticks or a fenced ``` block is taken out
of the draft before it is sent, and put back exactly as it was — which also means
it never leaves your machine and costs nothing to translate.

**Vim bindings.** `HERDR_POLYGLOT_VIM=1` makes the draft box modal, with the
motions, edits and counts that make sense inside a text box —
[the full list](docs/vim.md).

## Read the agent's reply in your language

The other direction: a third action, `wazum.polyglot.read`, translates what the
agent last said and shows it in a taller popup over the pane. Name the language
first — there is none to assume:

```bash
echo "HERDR_POLYGLOT_READ_LANGUAGE=DE" >> "$(herdr plugin config-dir wazum.polyglot)/.env"
```

```toml
[[keys.command]]
key = "prefix+r"
type = "plugin_action"
command = "wazum.polyglot.read"
description = "read the agent's reply in your language"
```

It goes through the same service a prompt does. For *Claude Code* and *Codex*
the replies come from the agent's own session file, one per prompt, without its
tool calls or its thinking — but with the questions *Claude Code* asks you and
the plans it asks you to approve, options numbered as the terminal numbers them; for any other agent the recent terminal output is
translated as one block. `HERDR_POLYGLOT_READ_REPLIES` says how many of the
latest replies are shown, and `HERDR_POLYGLOT_READ_CONTEXT` how many before them
go along so the same thing keeps the same word — *DeepL* takes them as its
unbilled context and *LM Studio* reads them in its prompt; *Google* and a
command translate each reply on its own. Code stays out, as it does for a prompt.

With *LM Studio* the translation streams in as the model writes it. A model that
thinks first shows its thinking, faded, while it goes on, and folds it into one
line — `▸ thought for 8s` — as soon as the answer begins. A reply opens at its
top, so it is read from the start; `G` goes to the end and follows the text as
it is written, until you scroll away. Other services answer all at once.

It can also open in a pane beside the agent, which leaves the keyboard where it
was, so you keep typing to the agent while you read. Bind both, and the key that
moves it between the two:

```toml
[[keys.command]]
key = "prefix+ctrl+r"
type = "plugin_action"
command = "wazum.polyglot.read-pane"

[[keys.command]]
key = "prefix+alt+r"
type = "plugin_action"
command = "wazum.polyglot.read-toggle"
```

- The pane key translates the latest reply again in the pane that is already
  open, dropping what it showed.
- `m` in either, or the toggle key, opens what is on show in the other place.

Closing a popup costs nothing to come back from: what it translated is kept,
and the popup key shows it again without asking the service. A reply still
being translated when its reader is moved is translated again by the one that
takes over; finished ones are not. herdr sizes a pane itself, so the reading
width and height apply to the popup only.

In the popup `↑ ↓ PageUp PageDown j k g G` scroll, as do `space` and `ctrl+b` a
page at a time, `tab` shows the reply as the agent wrote it, `M` shows it as
plain text or rendered as markdown again, the way glow does, `S` renders it in
the next of glamour's styles and keeps that one for the next popup, `y` copies
all of
what is on show (the translation, or the original after `tab`) to the clipboard,
`t` reads the agent's latest replies again and translates the new ones, `[` and
`]` step through the ones translated before,
`l` lists every reply of the session to pick one to translate,
`ctrl+t` tries again after a failure, `m` moves the reader, and
`esc` or `q` closes. Copying uses `pbcopy` on macOS, and `wl-copy`, `xclip` or
`xsel` on Linux.

Each translated reply is kept, the last 100 for each project: the repository the
agent works in, and every worktree of it on its own. A reply kept already is
shown at once, without asking the service again. A new one is not translated
until you press `t`: the popup opens on the latest translation kept, says a new
reply is waiting, and asks the service nothing. `t` also picks up what the agent
has said since the reader opened, so a pane left open beside it keeps up without
being opened again; a reply kept already is not translated twice. The pane key,
`ctrl+t` and `ctrl+o` translate anew straight away. A reply read before the agent finished it gives way to
the whole of it. `[` steps back through the earlier ones and `]` forward again,
past the newest back to the latest reply; `tab` and `y` work on them as on the
reply. The history is kept in the plugin's state directory and never sent
anywhere.

`l` lists every reply of the agent's session, oldest at the top, each under the
prompt it answered — a background task that ended goes by its summary — with a
✓ on the ones translated already. `p` or `→` shows the prompt and the reply
whole, as they were written, to scroll through before choosing, and `esc` goes
back to the list. `enter` translates the one picked, told the
replies before it as the latest would be; a ✓ one is shown at once. `l` and
`enter` on the newest goes back to the latest reply.

## What leaves your machine

With a service on the network named, the draft goes to it — and so do the
agent's replies you ask to read, with the ones before them that go along as context — so treat it the way
you treat anything you paste into a web translator: prompts for a coding agent carry file
paths, code and occasionally a secret, and in live mode the draft goes out again
after every pause in typing.

Code does not leave either: backticked spans and fenced blocks are held back and
restored afterwards, so a pasted stack trace or a file path is not sent anywhere.

Nothing else leaves. The API key goes to the translation service only — never to
the agent, the *herdr* socket, a command line or a child process. Translated
text is stripped of control characters before it is typed into a pane, so
neither a line break nor an escape sequence can reach the agent's terminal.
A [translator on the machine](docs/local-translation.md) keeps a draft off the
network entirely.

## Development

```bash
make qa     # formatting, linting, race tests, vulnerability scan
make build
herdr plugin link .
```

The overlay names palette slots rather than fixed colours, so it takes on
whatever *herdr* theme is active, including a light one.
[How the pieces fit together](docs/architecture.md), including how to add
another translation service.

## Credits

Created with ♥ by [Wolfgang Klinger](https://wolfgang-klinger.dev/).

Built on [*herdr*](https://herdr.dev),
[*Bubble Tea*](https://github.com/charmbracelet/bubbletea) and
[*Lip Gloss*](https://github.com/charmbracelet/lipgloss).

## License

[MIT](LICENSE).
