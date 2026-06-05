#!/usr/bin/env bash
# kit installer — installs Go + kit, wires PATH into your login shell's rc
# file, and runs `kit setup`. Safe to re-run (idempotent).
#
#   curl -fsSL https://raw.githubusercontent.com/andreicstoica/kit/main/install.sh | bash
#
# Why a script: `go install` drops kit in $(go env GOPATH)/bin, which isn't on
# PATH by default — so a bare `kit setup` would fail with "command not found".
# This wires PATH for future shells AND runs setup now via the absolute path,
# so it works in one shot regardless of your current PATH.
set -euo pipefail

info() { printf '\033[1;36m▸ %s\033[0m\n' "$1"; }
err()  { printf '\033[1;31m✗ %s\033[0m\n' "$1" >&2; }

# 1. Homebrew — needed for Go and the tools `kit setup` installs. If it's
#    installed but not on PATH (fresh Apple-Silicon installs don't auto-wire
#    it), load it from the canonical locations.
if ! command -v brew >/dev/null 2>&1; then
  for b in /opt/homebrew/bin/brew /usr/local/bin/brew; do
    if [ -x "$b" ]; then eval "$("$b" shellenv)"; break; fi
  done
fi
if ! command -v brew >/dev/null 2>&1; then
  err "Homebrew not found. Install it first, then re-run this:"
  echo '  /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"'
  exit 1
fi

# 2. Go.
if ! command -v go >/dev/null 2>&1; then
  info "installing Go via Homebrew…"
  brew install go
fi

# 3. kit.
info "installing kit (go install …@latest)…"
go install github.com/andreicstoica/kit@latest

# 4. Wire $(go env GOPATH)/bin into the login shell's rc file (idempotent).
#    Match kit's own fence so `kit setup` won't append a duplicate line.
gobin="$(go env GOPATH)/bin"
case "$(basename "${SHELL:-/bin/zsh}")" in
  bash) profile="$HOME/.bash_profile" ;;
  *)    profile="${ZDOTDIR:-$HOME}/.zshrc" ;;
esac
fence="# kit-setup: kit on PATH"
if [ -f "$profile" ] && grep -qF "$fence" "$profile"; then
  info "PATH already wired in $profile"
else
  info "adding $gobin to PATH in $profile"
  printf '\n%s\nexport PATH="%s:$PATH"\n' "$fence" "$gobin" >> "$profile"
fi

# 5. Run setup with PATH set for this process (so the in-process checks pass)
#    and via the absolute path (so it runs even though PATH isn't reloaded).
export PATH="$gobin:$PATH"
info "running kit setup…"
"$gobin/kit" setup

info "done — open a new terminal (or run: source $profile) so 'kit' is on PATH."
