# GitHub Copilot installer

An installer helper for the official
[GitHub Copilot application](https://github.com/github/app).
The application is downloaded from GitHub separately from the helper.

Check completion with `systemctl status github-copilot-installer.service` and
`github-copilot-installer status`. Start Copilot from the application menu or
run `github`.

Removing the helper removes the verified application, preserving user data
in `~/.copilot`. Helpers before 0.4.0 installed GitHub's RPM; remove it with
`dnf remove github` before installing this version.
