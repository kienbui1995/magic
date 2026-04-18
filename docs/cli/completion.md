# Shell Completion

The `magic` CLI ships with completion scripts for **bash**, **zsh**, and **fish**. Each script completes subcommand names (`serve`, `workers`, `tasks`, `submit`, `status`, `completion`, `version`, `help`) plus serve flags (`--config`) and completion shell arguments.

The scripts are emitted by the binary itself — no extra install artefact — so upgrading MagiC refreshes completion automatically.

## bash

System-wide:

```bash
magic completion bash | sudo tee /etc/bash_completion.d/magic > /dev/null
```

User-local (no sudo):

```bash
mkdir -p ~/.local/share/bash-completion/completions
magic completion bash > ~/.local/share/bash-completion/completions/magic
```

Then open a new shell (or `source` the file).

## zsh

```bash
magic completion zsh > "${fpath[1]}/_magic"
```

If you use a framework (oh-my-zsh, prezto, zinit), drop the file into its custom completions directory instead — e.g. `~/.oh-my-zsh/completions/_magic`. Reload with:

```bash
autoload -U compinit && compinit
```

## fish

```bash
magic completion fish > ~/.config/fish/completions/magic.fish
```

Fish picks up new completion files without reloading.

## Verify

Type `magic <TAB>` — you should see the seven subcommands. `magic serve --<TAB>` should offer `--config`. `magic completion <TAB>` should offer `bash / zsh / fish`.
