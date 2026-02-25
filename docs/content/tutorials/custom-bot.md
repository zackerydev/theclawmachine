---
title: Add a Custom Bot Type
weight: 4
---

Custom bot types are currently a **code change workflow**, not a runtime UI plugin.

## Required code changes

1. Add chart source under `control-plane/charts/<bot>/`.
2. Ensure packaging path emits `control-plane/internal/service/charts/<bot>.tgz`.
3. Add `//go:embed` entry and resolver handling in `internal/service/charts.go`.
4. Register bot metadata in `internal/botenv` so install/config UI has env metadata.
5. Verify install/onboarding handlers support the new bot type.

## Validate

```bash
mise run charts
mise run build
mise run test
```

Then run `clawmachine serve --dev` and verify bot appears on `/bots/new`.
