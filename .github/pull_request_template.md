## What

<!-- One sentence. What does this PR do? -->

## Why

<!-- What ticket/problem does this solve? Link the Linear issue. -->

## Scope

<!-- List key files/packages changed. -->

## How To Validate

<!-- Include exact commands and outcomes. -->

### Required

- [ ] `mise run test:all`
- [ ] `mise run vet`
- [ ] `mise run lint`
- [ ] `mise run fix`
- [ ] `mise run docs:check` (if docs or user-facing behavior changed)

### Optional / When Relevant

- [ ] `mise run e2e:setup && mise run e2e:run` (lifecycle/kind changes)
- [ ] `mise run e2e:teardown` or `mise run e2e:cleanup` after lifecycle tests

## Evidence

<!-- Paste key output snippets, screenshots, or links. -->

## Checklist

- [ ] Tests added/updated
- [ ] Routes/CLI/docs updated together when contract changed
- [ ] No stale terminology introduced
- [ ] Local validation run (not just compile)
