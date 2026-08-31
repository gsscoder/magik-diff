# Build Directory

The build directory holds all build files and assets for the application.

The structure is:

* bin - Output directory
* darwin - macOS specific files
* windows - Windows specific files

## Mac

The `darwin` directory holds files specific to Mac builds.
These can be customised for the build. To restore them to their default state, delete them and run `wails build`.

The directory contains the following files:

- `Info.plist` - the main plist file used for Mac builds. It is used when building using `wails build`.
- `Info.dev.plist` - same as the main plist file but used when building using `wails dev`.

## Windows

The `windows` directory contains the manifest and rc files used by `wails build`.
These can be customised for the application. To restore them to their default state, delete them and run `wails build`.

- `icon.ico` - The application icon, used when building with `wails build`. Replace it to use a different icon; if it is missing, one is generated from `appicon.png` in the build directory.
- `installer/*` - Files used to create the Windows installer, consumed by `wails build`.
- `info.json` - Application metadata for Windows builds, used by the installer and visible in the executable's Properties → Details tab.
- `wails.exe.manifest` - The main application manifest file.