The build script (`build/build.ps1` or `build/build.sh`) puts the files the installer embeds here:

- `GradeStream.exe` — the app built from `app/`;
- `cloudflared.exe` — the official Cloudflare-signed cloudflared for Windows (downloaded from Cloudflare's releases);
- `gradestream.ico` — the icon (copy of `docs/gradestream.ico`).

These files are not committed to the repository.
