# Claude Desktop installer

Install Anthropic's official Linux application on Fedora 44 x86_64 through
an open-source helper RPM. The helper downloads the application from Anthropic;
COPR hosts only the helper. Anthropic does not support Fedora.

Check completion with `systemctl status claude-desktop-installer.service` and
`claude-desktop-installer status --json`. Launch Claude from the application
menu or run `claude-desktop`.

New helper releases select application updates. Removing the helper removes
the verified application and preserves user settings. Cowork needs a paid plan,
working virtualization and access to `/dev/kvm`.
