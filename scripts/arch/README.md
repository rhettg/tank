# Arch packaging

This directory contains starter files for the Arch Linux package.

## Files

- `PKGBUILD`: Builds `tank` from the Git tag and installs shared layers, the
  UFW profile, and Arch install hooks.
- `tank.install`: Runs post-install/post-remove steps similar to the Debian/RPM
  packages (create `/var/lib/tank`, configure `virbr0`, and set UFW rules when
  UFW is active).

## Notes

- Update `pkgver` to the tag you are packaging.
- Copy `tank.install` into the build directory; the `prepare()` hook handles
  this when using `makepkg`.
- Replace the `sha256sums` entry with the actual checksum.
- The `.install` script is installed by `PKGBUILD` and run by `pacman`.
