#!/bin/sh
# Generates shell completion scripts for the release archives and the Homebrew cask.
set -eu
rm -rf completions
mkdir -p completions
for sh in bash zsh fish; do
  go run ./cmd/hearthroom completion "$sh" > "completions/hearthroom.$sh"
done
