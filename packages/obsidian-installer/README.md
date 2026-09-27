# Obsidian installer

Install and update the official Obsidian AppImage through the helper RPM.
The application is downloaded from Obsidian separately from the helper.

Check completion with `systemctl status obsidian-installer.service` and
`obsidian-installer status --json`. Start Obsidian from the application menu
or run `obsidian`.

New helper releases update the AppImage and its Electron runtime. Obsidian
can also update its application code through its own automatic updates.
Removing the helper removes the verified application, preserving vaults
and user settings. Existing Flatpak installations are not removed.
