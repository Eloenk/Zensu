#!/bin/bash
# Script to assemble a Debian package (.deb) for Zensu
set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
BASE_DIR="$DIR/../.."
PKG_DIR="$DIR/zensu-amd64"

echo "Assembling Debian package structure..."
rm -rf "$PKG_DIR"
mkdir -p "$PKG_DIR/DEBIAN"
mkdir -p "$PKG_DIR/usr/bin"
mkdir -p "$PKG_DIR/usr/share/applications"
mkdir -p "$PKG_DIR/usr/share/pixmaps"

# Write DEBIAN/control file
cat <<EOT > "$PKG_DIR/DEBIAN/control"
Package: zensu
Version: 1.0.0
Section: utils
Priority: optional
Architecture: amd64
Maintainer: Google Deepmind Antigravity Team
Depends: libgtk-3-0, libwebkit2gtk-4.0-37, ffmpeg
Description: Premium Anime Downloader built with Go and Wails.
EOT

# Copy compiled binaries
cp "$BASE_DIR/build/bin/zensu" "$PKG_DIR/usr/bin/zensu"
cp "$BASE_DIR/build/bin/cli/zensu-cli" "$PKG_DIR/usr/bin/zensu-cli"

# Copy shortcut file and app icon
cp "$DIR/zensu.desktop" "$PKG_DIR/usr/share/applications/zensu.desktop"
cp "$BASE_DIR/build/appicon.png" "$PKG_DIR/usr/share/pixmaps/zensu.png"

# Set permissions
chmod -R 755 "$PKG_DIR"
chmod 755 "$PKG_DIR/usr/bin/zensu"
chmod 755 "$PKG_DIR/usr/bin/zensu-cli"

# Build package
echo "Building .deb package..."
dpkg-deb --build "$PKG_DIR" "$BASE_DIR/build/bin/zensu.deb"

# Clean up
rm -rf "$PKG_DIR"
echo "Debian package zensu.deb built successfully at build/bin/zensu.deb!"
