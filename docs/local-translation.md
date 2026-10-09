# Translation on your own machine

Two ways keep a draft off the network: a language model served by
[LM Studio](#lm-studio), which is what the plugin uses when nothing else is
configured, and [any program](#a-program-on-the-machine) that reads text and writes
its translation.

## LM Studio

[LM Studio](https://lmstudio.ai) serves the model it has loaded at
`http://localhost:1234`. Load a model, then start the server — in the app's
*Developer* tab, or from a terminal:

```bash
lms server start
lms load qwen/qwen3-8b
```

That is the whole configuration: with no key, command or service named, the plugin
sends the draft there and uses whichever model is loaded. To say it out loud, or to
pick a model:

```bash
ENV_FILE="$(herdr plugin config-dir wazum.polyglot)/.env"
echo 'HERDR_POLYGLOT_PROVIDER=lmstudio' >> "$ENV_FILE"
echo 'HERDR_POLYGLOT_MODEL=qwen/qwen3-8b' >> "$ENV_FILE"
```

`HERDR_POLYGLOT_LANGUAGE` names the target language as it does for any service.
The draft goes to the model inside `<text>` tags, with instructions to translate
it and never to answer it — a prompt for an agent is a question, and a model given
one plainly tends to reply. Code in backticks is taken out before it is sent, as
with every service.

Three settings adjust what is sent, for a model that needs it:

| Setting | Goes | For example |
| --- | --- | --- |
| `HERDR_POLYGLOT_SYSTEM_PROMPT_EXTRA` | after the instructions | `Use British spelling. Keep the German term Mandant.` |
| `HERDR_POLYGLOT_USER_PROMPT_EXTRA` | after the draft, outside the tags | `/no_think`, which stops a *Qwen 3* model reasoning first |
| `HERDR_POLYGLOT_REQUEST_EXTRA` | into the request, replacing what is there | `{"max_tokens": 2048, "top_p": 0.9}` |

The request setting is a JSON object on one line; it may not set `messages` or
`stream`, which carry the draft and the answer. A reasoning model's
`<think>…</think>` is left out of the translation.

Anything else that answers OpenAI-style chat completions — *Ollama*,
`llama-server`, *vLLM* — works the same way with `HERDR_POLYGLOT_ENDPOINT` set to
its `/v1/chat/completions` URL. A server elsewhere on your network is allowed over
plain `http`; with a key set it must be `https` or on this machine.

The first request after LM Studio starts loads the model, which can take most of a
minute. If the footer says `LM Studio did not answer in time — is the model still
loading?`, wait for it and press `ctrl+t`. Once loaded, a short prompt comes back in
about a second, depending on the model's size.

## A program on the machine

`HERDR_POLYGLOT_COMMAND` points the plugin at a program instead of a service. The
draft is written to that program's input, and what it writes back is the
translation. No key, no account, no network.

```bash
ENV_FILE="$(herdr plugin config-dir wazum.polyglot)/.env"
echo 'HERDR_POLYGLOT_COMMAND=/Applications/translateLocally.app/Contents/MacOS/translateLocally -m de-en-base' >> "$ENV_FILE"
```

That is the whole configuration: a command and no key means the command
translates, so `HERDR_POLYGLOT_PROVIDER` can stay unset.

### translateLocally

[translateLocally](https://translatelocally.com) runs the *Bergamot* models
*Firefox* uses for its own translation, and it is quick enough for live
translation — a prompt of a couple of sentences comes back in well under a second
on an M-series Mac. Install it, then download the model for your language:

```bash
translateLocally --available-models
translateLocally --download-model de-en-base
translateLocally --list-models
```

The model name carries the direction, so `-m de-en-base` says what
`HERDR_POLYGLOT_LANGUAGE` says for a service. The setting still names the target
language for the header, and the command can read it as
`$POLYGLOT_TARGET_LANGUAGE` if it would rather be told.

On macOS the binary lives inside the application bundle, at
`/Applications/translateLocally.app/Contents/MacOS/translateLocally`. On Linux it
is on the path.

### Anything else that reads and writes text

The command line goes to a shell, so a pipeline, flags and quoting all work:

```bash
HERDR_POLYGLOT_COMMAND=curl -s -X POST http://localhost:5000/translate -H 'Content-Type: application/json' --data-binary @- ...
HERDR_POLYGLOT_COMMAND=trans -brief -no-warn :en
HERDR_POLYGLOT_COMMAND=ollama run translategemma
```

Only two rules: read the draft from standard input, and write the translation to
standard output. Anything on standard error is what the popup shows when the
command fails, so a program that explains itself there explains itself in the
footer.

## What the popup says when it goes wrong

| It says | It means |
| --- | --- |
| `LM Studio could not be reached` | Its server is not running, or not on that endpoint |
| `LM Studio refused the draft: …` | It runs but would not translate — no model loaded, usually; the rest says why |
| `LM Studio did not answer in time — is the model still loading?` | A minute passed with no answer |
| `translateLocally could not be started — is it installed?` | The shell could not find or execute it; check the path |
| `translateLocally failed: <what it complained about>` | It ran and refused — a missing model, usually |
| `translateLocally did not answer in time` | A minute passed with no answer; it is killed, and the rest of its pipeline with it |
| `translateLocally returned nothing` | It succeeded and wrote nothing, which would deliver an empty prompt |

`ctrl+t` tries again once the reason is fixed, without retyping the draft.

## What it costs

Nothing, and nothing leaves the machine. The sentence cache and the code
protection still work the same way, so a long draft is not re-translated word by
word while you write — see [live translation](live-translation.md).

The trade is quality: these models are small, and a long sentence with subclauses
comes back flatter than *DeepL* would put it. For prompts to a coding agent that
is usually enough, and you read the English beside your draft anyway.
