# How it works

The keybinding runs an action that knows which pane you pressed it in. The
action opens the draft box as a floating popup over that pane and passes the
pane id along in the environment, so the prompt lands where it was written.
Sized popups are only reachable over *herdr*'s socket API, not its CLI, so the
plugin speaks that protocol itself for this one call.

On `alt+enter` the draft goes to a translation service and the result is handed
back to the same pane through the *herdr* CLI: `agent prompt` to send it, or
`pane send-text` to type it without sending.

## The pieces

| Package | Holds |
| --- | --- |
| `promptflow` | The use case: translate a draft, deliver a prompt. Owns nothing about terminals. |
| `translation` | The `Translator` and `Provider` ports, the registry of services, the sentence-level cache used for previews, and the decorator that keeps code out of a translation. |
| `deepl`, `google`, `command`, `lmstudio` | Services behind those ports: two hosts, one program on the machine and a language model served on it. |
| `herdr` | Talking to *herdr*: the CLI runner, the two delivery targets, the socket. |
| `transcript` | What the agent in a pane replied: its *Claude Code* or *Codex* session file, split into one reply per prompt, or else the terminal's recent output. |
| `overlay` | The *Bubble Tea* program: the draft box, the header and footer, the keys. |
| `vimarea` | A text area with modal editing, used by the overlay. |
| `config` | Settings from the environment and the plugin's `.env`. |
| `draft` | An unfinished prompt on disk, one file per pane, written privately and atomically. |

`cmd/herdr-polyglot` is the composition root: it reads the settings, picks the
service, wires the flow to its targets and starts the program. Nothing below it
knows which service is in use or how the popup was opened.

`promptflow` owns the ports it needs — `Translator`, `Target`, `UsageReporter` —
and imports no adapter package, not even `translation`. Where the two
vocabularies have to meet, they meet in the composition root: it is the only
place that knows both a `translation.Usage` and a `promptflow.Usage` exist, and
it adapts one to the other. `promptflow.New` assumes the dependencies it is
handed are real; the composition root is its only caller, and a nil would fail
on the first keystroke.

Previews and sends use different translators. A send is one shot and goes to the
service directly; a preview goes through the sentence cache, because writing
means translating the same draft again and again. Both are set up whether live
translation starts on or off, since `ctrl+l` can turn it on at any time.

The draft is stored when the program ends, whichever way it ended: *herdr*
closes a popup by hanging up on it, and `ctrl+c` never passes the close key
either, so the last word on the draft cannot be a keystroke.

## Reading the agent's replies

`open --read` opens the same popup entrypoint with `HERDR_POLYGLOT_MODE=read`, and
`run` hands over to `runReader` before anything to do with a draft is built. It
asks `herdr agent get` which agent and session the pane holds, and `transcript`
finds the session file by its id. The service is chosen as for a prompt, but built
a second time with `HERDR_POLYGLOT_READ_LANGUAGE` as its target, since a provider
fixes its language when it is made.

Each shown reply is translated on its own, told what came before it: the context
replies and any shown reply ahead of it, the last 4,000 characters. `Protecting`
passes that along to a `ContextualTranslator` with its code left out, and
translates the reply alone for a service that is not one.

A service that writes its answer bit by bit also implements
`StreamingTranslator`, and reports a `Progress` — the thinking so far, the
translation so far, whether the thinking is over — after every piece. Each
progress holds everything before it, so the reader keeps only the newest one
waiting and a slow screen skips pieces without losing any. `Protecting` shows a
marker as its code as soon as the marker has arrived whole, and checks every
marker only once the answer is complete. `lmstudio` streams, and allows its
timeout for each silence rather than for the whole answer.

## Another translation service

*DeepL* is one implementation of a small interface, not a dependency of the design:

```go
type Provider interface {
	Name() string
	New(Options) (Translator, error)
}

type Translator interface {
	Translate(ctx context.Context, draft string) (string, error)
}
```

A service that can be told what came before a sentence — and does not bill for
it — also implements `ContextualTranslator`, which is what makes live
translation cheap:

```go
type ContextualTranslator interface {
	TranslateWithContext(ctx context.Context, text, preceding string) (string, error)
}
```

`translation.Options` is a superset — `APIKey`, `TargetLanguage`, `Endpoint`,
`Command` — and each service takes what applies. That holds while services want the same
three things. The moment one needs something private to it, a project id or a
region, that parsing belongs in the composition root rather than in a struct
every service has to look at: adding fields there makes every provider carry
what one of them needed. `lmstudio` is that case: its model and prompt settings
are fields of `lmstudio.Provider`, filled in `services()`, and `chooseService`
refuses them when another service is chosen.

Add a package that implements `Provider`, register it in `services()` in
`cmd/herdr-polyglot/main.go`, and it becomes selectable through
`HERDR_POLYGLOT_PROVIDER`. The first registered service is the default, and it
must be one that keeps the draft on the machine: `chosenName` never picks a
service on the network unless it is named. A
service that reports what it has spent can implement `UsageReporter`, and the
header shows the count.

## Working on it

```bash
make qa     # formatting, linting, race tests, vulnerability scan
make build
herdr plugin link .
```

Tests are written from the outside in and named as sentences about behaviour.
The overlay is driven through `teatest`, which means its tests type keys and
read frames rather than reaching into the model.
