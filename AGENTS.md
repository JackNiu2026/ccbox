# CCBox repository instructions

- This is CCBox (JackNiu2026/ccbox). Use the current CCBox checkout as the source of truth. Treat Cage and agent-vm upstream changes as reference material and port only the relevant changes. Do not replace CCBox source, UI, branding, or installers with upstream files or run upstream installers to repair CCBox.
- The CCBox backend entry point is `./cmd/cage`. Build the desktop app with `scripts/build-windows.ps1`, `scripts/build-app.sh`, or `scripts/build-linux.sh`. Use the CCBox packaging scripts for installers. The official public downloads and update feed are under https://ccbox.org/.
- Preserve the shipped name CCBox and the existing application and installer identities. Internal names such as `cage`, `ccBox.install-id`, `org.ccbox.desktop`, `CageData` and `/etc/claude-cage` are compatibility interfaces. Do not rename them as part of a branding change.
- Desktop updates replace host application files only. Reuse the saved profile, disk identity and paths, including BoxData, legacy CageData and custom storage. Never reset, re-clone, delete or provision over an existing environment to make an app upgrade pass. Guest component and tool maintenance is a separate explicit operation.
- Run `node scripts/check-ccbox-brand.mjs` and the affected tests before proposing a change. Keep upstream license and attribution notices intact.
