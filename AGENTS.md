# Agent notes

- Use Go 1.24+ for builds (see `go.mod`).
- Run `gofmt` on any Go files you touch.
- Keep labels explicit and namespaced; update `README.md` when label semantics change.
- Preserve the controller pattern: Docker adapter, label parser, Cloudflare client, reconciler.
- Avoid hidden defaults or implicit behavior; log warnings for skipped actions.
- Never add write access to the Docker socket without explicit user request.
