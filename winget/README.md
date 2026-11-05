# Publishing LNReaderTUI to Windows Package Manager (winget)

LNReaderTUI is a portable Windows `.exe` published to GitHub Releases, so it **can** be published to the
[community winget-pkgs](https://github.com/microsoft/winget-pkgs) repository. The package identifier is
`hhdtc.LNReaderTUI` (publisher must match the GitHub owner).

This folder holds a ready-to-submit multi-file manifest set plus the repo changes that make the release
pipeline emit the artifact winget needs.

## What is set up

- **`.github/workflows/release.yml`** now also produces `lnreadertui-windows-amd64.zip` (containing
  `lnreadertui.exe`) for every tagged release, so winget can expose a clean `lnreadertui` command.
- **`winget/manifests/h/hhdtc/LNReaderTUI/1.1.6/`** — the manifest set using
  `InstallerType: zip` + `NestedInstallerType: portable` + `PortableCommandAlias: lnreadertui`.

```
winget/manifests/h/hhdtc/LNReaderTUI/1.1.6/
├── hhdtc.LNReaderTUI.yaml                  # version manifest
├── hhdtc.LNReaderTUI.installer.yaml        # installer manifest (zip + portable + alias)
└── hhdtc.LNReaderTUI.locale.en-US.yaml     # default (en-US) locale manifest
```

> The community repo forbids singleton manifests — submit the full multi-file set.

## Keeping the manifest current

The manifest directory here mirrors what was submitted upstream and carries the real released hashes, so
each new version follows the same three steps:

1. **Cut the release first** — `git tag vX.Y.Z && git push origin vX.Y.Z` builds `dist/*`, including
   `lnreadertui-windows-<arch>.zip`.
2. **Take the zip's SHA-256** from the release asset digest (or `sha256sum` after downloading) and write it
   into `winget/manifests/h/hhdtc/LNReaderTUI/vX.Y.Z/hhdtc.LNReaderTUI.installer.yaml`.
3. **Open the upstream PR** (see below) with the new version directory.

`hhdtc.LNReaderTUI` is already in winget-pkgs, so every PR after the first one is an *update*: add the new
version directory and leave the older ones untouched. Title it `Update: hhdtc.LNReaderTUI to X.Y.Z`
(the first submission used `New package: hhdtc.LNReaderTUI version X.Y.Z`). The `New-Manifest` label the
bot applies is about the new manifest *files*, not about the package being new.

`1.1.6` ships `x64` and `arm64` installers, both verified against the published release assets.

## Steps to submit to winget-pkgs

1. Confirm there is no other **open PR** for the same package/version.
2. Fork `microsoft/winget-pkgs` and copy exactly `manifests/h/hhdtc/LNReaderTUI/1.1.6/` into the fork.
3. Validate locally:
   ```powershell
   winget validate --manifest winget/manifests/h/hhdtc/LNReaderTUI/1.1.6
   ```
4. Test the install (run as Administrator):
   ```powershell
   winget settings --enable LocalManifestFiles
   winget install --manifest winget/manifests/h/hhdtc/LNReaderTUI/1.1.6
   ```
   > ⚠️ LNReaderTUI is a bubbletea TUI and needs a VT terminal such as **Windows Terminal**; it will not
   > render correctly in classic `cmd.exe`. The install still succeeds; this only affects how it looks.
5. Open the PR against `microsoft/winget-pkgs` (base branch `master`) containing only the manifest files,
   then fix any validation labels the bot applies.

## "If I publish a new version on GitHub, will winget get it?"

**No — not by default.** `winget install` resolves against the winget-pkgs repository, which stores a
fixed `PackageVersion` + `InstallerSha256` per package. A new GitHub tag/release does **not** touch
winget. Each version needs its own winget-pkgs PR (or automation to open one).

To make it automatic, add a workflow that runs on each release and opens/updates the winget-pkgs PR. A
common choice is `vedantmgoyal2009/winget-create` (or `russellbanks/Komac`), e.g.:

```yaml
name: Submit to winget
on:
  release:
    types: [published]
permissions:
  contents: write
jobs:
  submit:
    runs-on: ubuntu-latest
    steps:
      - name: wingetcreate
        uses: vedantmgoyal2009/winget-create@v2
        with:
          identifier: hhdtc.LNReaderTUI
          version: ${{ github.event.release.tag_name }}
          installers-regex: 'lnreadertui-windows-amd64\.zip$'
          release-tag: ${{ github.event.release.tag_name }}
          token: ${{ secrets.WINGET_PAT }}
          fork-user: hhdtc
```

**Setup required:**
- Add a fine-grained PAT as the `WINGET_PAT` repository secret. It needs `repo` scope and permissions to
  push to your fork (`hhdtc/winget-pkgs`) and open PRs.
- `fork-user` should be your GitHub username (`hhdtc`) so the PR comes from your fork.

With that in place, every published release opens/updates the winget-pkgs PR automatically. Until it is
added, a new release only requires you to re-run steps 3–5 above (update the version + ZIP SHA-256 and open
a new PR).

## Caveats

- **Security scans / PUA risk.** Every winget-pkgs package is scanned (VirusTotal etc.). LNReaderTUI uses
  `fhttp` / `tls-client` to fingerprint Chrome TLS and scrapes bilinovel.net. Inspectors may flag the
  fingerprint-spoofing behavior even though the app is legitimate; acceptance is not guaranteed.
- **Publisher match.** `Publisher` must match the publisher segment of the identifier (`hhdtc`).