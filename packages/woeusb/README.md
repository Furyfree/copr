# WoeUSB

Fedora 44 package for [WoeUSB](https://github.com/WoeUSB/WoeUSB), a command-line
tool for creating bootable Windows installation USB media from an ISO or
installation disc.

The package installs the upstream 5.2.4 command as `/usr/bin/woeusb`, its man
page, and the runtime dependencies needed for FAT32/NTFS media creation. In
particular it installs `grub2-pc-modules` alongside `grub2-tools`, so the
legacy-PC bootloader stage has the Fedora i386-pc modules it expects.

## Install

```sh
sudo dnf copr enable furyfree/woeusb
sudo dnf install woeusb
```

Then use the normal upstream command:

```sh
woeusb --help
sudo woeusb --device Windows.iso /dev/sdX
```

`--device` destroys the existing contents of the selected target disk. Verify
the device before running it.

FAT is WoeUSB's default target filesystem. If `install.wim` exceeds FAT32's
single-file size limit, WoeUSB uses `wimlib-imagex` to split it into SWM parts
that Windows Setup can consume.

## Automounters

An automounter can race WoeUSB after it creates the new target partition. If a
tool such as `udiskie` immediately mounts the partition, WoeUSB can fail with
`contains a mounted filesystem`. Stop the automounter for the write and start
it again after WoeUSB exits.

## Packaging

The recipe pins and verifies the upstream 5.2.4 standalone script, man page and
GPL-3.0-or-later license before building. The RPM is noarch; this COPR project
currently builds it for Fedora 44 x86_64.
