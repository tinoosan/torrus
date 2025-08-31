Title: Allow injecting custom logger into aria2 Adapter

Summary
- Add a way to supply a custom `*slog.Logger` to the aria2 downloader adapter so callers can unify logs with their application logger instead of relying on `slog.Default()`.

Motivation
- Services embedding Torrus often set up their own structured logger with specific handlers/levels/attrs. Allowing injection avoids mixed formats and enables richer context (request IDs, component fields).

Proposal
- Add an optional constructor and/or setter:
  - `func NewAdapterWithLogger(cl *aria2.Client, rep downloader.Reporter, log *slog.Logger) *Adapter`
  - or `func (a *Adapter) WithLogger(log *slog.Logger) *Adapter` returning `a` for chaining.
- Keep `NewAdapter(...)` as-is, defaulting to `slog.Default()` to preserve current behavior.
- Use the injected logger for internal adapter logs (e.g., `tellStatus` errors).

Scope
- No behavioral changes to download flow.
- No change to Reporter or Event types.
- No persistence changes.

Acceptance Criteria
- New constructor/setter exists and compiles.
- Existing tests pass without changes.
- When a custom logger is provided, adapter uses it for warnings instead of the default logger.

Notes
- Future: consider surfacing a more general adapter options struct (`AdapterOptions{Logger, PollMS, ...}`) to avoid constructor sprawl.

