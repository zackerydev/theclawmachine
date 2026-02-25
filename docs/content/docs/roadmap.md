---
title: Roadmap
weight: 160
prev: development
---

What's coming to ClawMachine. This is a living list — items may shift in priority as the project grows.

## Planned

**Egress allow-list helpers**
Pre-populated blocks of known egress endpoints for common bot platforms (Discord, Telegram, Anthropic, OpenAI, etc.) so you don't have to look up and type URLs manually. Select your integrations, get the right domains in the network policy automatically.

**Full terminal editor in CLI tab**
Replace the current single-line input in the bot CLI tab with a proper embedded terminal experience — scrollback, multi-line input, and live output streaming.

**Better JSON config editor**
The current config editor is a raw textarea. A structured editor with syntax highlighting, inline validation, and schema-aware autocomplete is on the list.

**Cilium webhook integration for blocked requests**
Surface Cilium's policy verdicts directly in the Network tab. When a bot's egress request is blocked, show it in real time — destination, port, and the policy rule that dropped it — instead of requiring `kubectl` to diagnose.

**Generate CLI docs directly from Cobra**
Replace manually maintained command documentation with generated CLI reference output from Cobra so command docs stay aligned with the binary automatically.

---

Have a feature request? [Open an issue on GitHub](https://github.com/zackerydev/clawmachine/issues).
