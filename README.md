# fs

Serve a folder as a website, or share it with your network, with a single command.

```sh
fs C:\www
```

Open the address it prints. If the folder has an `index.html`, you see your
website. Otherwise, you get a file explorer for browsing and downloading files.

## Install

Build it with [Go](https://go.dev/dl) 1.24 or newer, from the project folder:

```sh
go build -o fs.exe .     # Windows
go build -o fs .         # Linux / macOS
```

The result is a single file with no dependencies. Copy it anywhere.

## Usage

```
fs [options] <folder>
```

| Option | Description |
| --- | --- |
| `-p`, `--port <port>` | Port to use (default `80`) |
| `--host <address>` | Only listen on this address, e.g. `127.0.0.1` for this computer only |
| `--no-index` | Always show the file explorer, even if a folder has an `index.html` |
| `--auth <user:pass>` | Require a username and password |
| `--verbose` | Print a line for every request |
| `-v`, `--version` | Show the version |
| `-h`, `--help` | Show help |

Examples:

```sh
fs ./site                         # https://localhost:80
fs --port 8080 C:\www             # https://localhost:8080
fs --port 443 --auth me:secret D:\shared
```

On startup, fs shows the addresses to open, including the ones other devices
on your network can use. Press **Ctrl+C** to stop.

## What it does

- **Hosts websites.** Folders with an `index.html` are served as websites, with
  the correct file types for HTML, CSS, JavaScript, fonts, images, video and more.
  Changes on disk show up when you refresh.
- **File explorer.** Browse folders, see file and folder sizes, sort by name,
  size or date, filter by typing, and download any file. Folders download as
  a ZIP.
- **Video and large files.** Videos can be seeked, and interrupted downloads
  can resume.
- **Always HTTPS.** fs creates its own certificate on first start. `http://`
  addresses are redirected to `https://` automatically.
- **Works from other sites.** Web apps running elsewhere, such as a dev server
  on another port, can load your files (CORS is enabled).
- **Only that folder.** Nothing outside the folder you choose can be accessed.
  Everything inside it, including hidden files, is visible to anyone who can
  connect, so use `--auth` for anything private.
- **Read-only.** Visitors can view and download, but never upload, change or
  delete anything. Server-side code such as PHP is not run. Those files are
  downloaded as plain text.

## The browser warning

Because fs makes its own certificate, browsers warn the first time you open it.
Click **Advanced → Continue** to proceed. To make the warning go away for good on
Windows (Edge and Chrome), run this once:

```powershell
certutil -user -addstore Root "$env:APPDATA\fs\cert.pem"
```

In Firefox, or on other devices, import `cert.pem` from that folder as a trusted
certificate authority. fs keeps the same certificate between runs. It only
makes a new one when it expires or your network address changes.

If you have your own certificate, for example from Let's Encrypt or mkcert,
replace `cert.pem` and `key.pem` in that folder with your own files. On macOS the
folder is `~/Library/Application Support/fs`. On Linux it's `~/.config/fs`.

## Troubleshooting

- **"port 80 is already in use"**: another program (such as IIS or another web
  server) is using it. Stop it or choose another port: `--port 8080`.
- **"access denied" / "permission denied"**: on Linux, ports below 1024 need
  `sudo`. On Windows, the port may be reserved by the system. Use another port.
- **Other devices can't connect**: allow fs through the firewall when Windows
  asks (Private networks), and use the "Network" address fs prints.
