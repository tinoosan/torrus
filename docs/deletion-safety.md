# Deletion Semantics & Safety

This document details how Torrus removes payload files and control sidecars when deleting a download with `deleteFiles=true`.

## Safety Principles

- Never delete the base `targetPath` itself; only operate on paths strictly under `targetPath/`.
- Normalize all candidate paths and verify they remain within `targetPath/`.
- Deduplicate delete candidates to avoid repeated filesystem calls and duplicate log lines.
- Prune empty parent directories deepest-first after payload removal.
- Best‑effort removal of a root `.aria2` occurs only when the pruned directory is proven to be the logical root of the download.
- Symlinks: if a candidate is a symlink, remove the link only; do not touch the symlink target.

## Sidecar Ownership Rules

Sidecar files are `.aria2` and `.torrent`. Torrus removes them only when ownership can be proven by one of the rules below.

1) Exact name match

- Sidecar filename (without extension) equals the download `Name`.
- Examples: `base/<Name>.aria2`, `base/<Name>.torrent`.
- If the logical root directory equals `base/<Name>`, then `root.aria2` and `root.torrent` are also removed.

2) Adjacent to known payload

- `<file>.aria2` is safe to delete if `<file>` is a payload path that originated from `aria2.getFiles` or `dl.Files` for this download.

3) Trimmed leading‑tags (best‑effort)

- If the `Name` begins with bracketed tags (e.g. `[ABC] My.Show.S01`), and the trimmed value matches a candidate root, delete `<trimmed>.aria2`/`.torrent` only with strong proof:
  - The matching sidecar file already exists, or
  - At least two distinct basenames in `dl.Files` appear under the candidate folder.

If none of the above rules match, sidecars remain untouched.

## Rationale

These rules minimize the risk of deleting unrelated control files in shared directories while still cleaning up legitimate sidecars created for the deleted download.
