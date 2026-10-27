# fs

Serve a folder as a website, or share it with your network, with a single
command. For Windows.

```powershell
fs C:\www
```

Open the address it prints. If the folder has an `index.html`, you see your
website. Otherwise, you get a file explorer for browsing and downloading files.

## Install

Download `fs-<version>-windows-amd64.exe` from the releases page, or
`windows-arm64` for ARM laptops. Rename it to `fs.exe` and put it in a folder
on your PATH.

To build it yourself, use [Go](https://go.dev/dl) 1.24 or newer, from the
project folder:

```powershell
go build -o fs.exe .
```

## Usage

```
fs [options] [folder]          serve a folder in this terminal
fs start [options] [folder]    serve a folder in the background (same as fs -d)
fs ps                          list running services
fs stop [id...]                stop the given services, or all of them
fs logs [-f] <id>              show a background service's log
```

The options below work the same with `fs` and `fs start`, e.g.
`fs start --verbose -p 8080 C:\www`.

| Option | Description |
| --- | --- |
| `-p`, `--port <port>` | Port to use (default `80`) |
| `--host <address>` | Only listen on this address, e.g. `127.0.0.1` for this computer only |
| `--no-index` | Always show the file explorer, even if a folder has an `index.html` |
| `--auth <user:pass>` | Require a username and password |
| `-d`, `--detach` | Run in the background |
| `--verbose` | Log every request, not just warnings and errors |
| `--set-default-path <folder>` | Save the folder `fs` serves when you don't give one |
| `-v`, `--version` | Show the version |
| `-h`, `--help` | Show help |

On startup, fs shows the addresses to open, including the ones other devices
on your network can use. In the foreground, press **Ctrl+C** to stop.

### Running in the background

Run as many folders as you like in the background, each on its own port,
without keeping a terminal open for each:

```powershell
fs start -p 8080 C:\www
fs start -p 8081 --verbose --auth me:secret D:\shared
fs ps
```

```
ID   FOLDER      URL                      LOGIN       INDEX   MODE         PID     STATUS
1    C:\www      https://localhost:8080   -           on      background   10432   Up 5m
2    D:\shared   https://localhost:8081   me:secret   on      background   11876   Up 12s
```

```powershell
fs logs 2       # show service 2's log (add -f to keep following it)
fs stop 1       # stop service 1
fs stop         # stop everything
```

Services keep running after you close the terminal. Services started in the
foreground show up in `fs ps` too, and `fs stop` stops them as well. IDs start
at 1 and count up. Once nothing is running, they start at 1 again.

A service's log holds its startup message, warnings and errors, and with
`--verbose` every request as well. Logs only exist while a service runs.
`fs stop` deletes them, and logs of services that ended some other way, for
example when Windows shut down, are cleaned up the next time you use fs.
`fs logs -f` ends by itself when the service stops.

### Default folder

Set a default once, then leave the folder out:

```powershell
fs --set-default-path C:\www
fs                  # serves C:\www
fs start            # serves C:\www in the background
fs D:\other         # a folder you pass still wins
```

## What it does

- **Hosts websites.** Folders with an `index.html` are served as websites, with
  the correct file types for HTML, CSS, JavaScript, fonts, images, video and more.
  Changes on disk show up when you refresh.
- **File explorer.** Browse folders, see file and folder sizes and how many
  files and sub-folders each folder holds, sort by name, size or date, filter by
  typing, and download any file. Folders download as a ZIP.
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
Click **Advanced → Continue** to proceed. To make the warning go away for good in
Edge and Chrome, run this once:

```powershell
certutil -user -addstore Root "$env:APPDATA\fs\cert.pem"
```

In Firefox, or on other devices, import `cert.pem` from that folder as a trusted
certificate authority. fs keeps the same certificate between runs. It only
makes a new one when it expires or your network address changes.

If you have your own certificate, for example from Let's Encrypt or mkcert,
replace `cert.pem` and `key.pem` in `%AppData%\fs` with your own files.

## Where fs keeps its files

Everything is in `%AppData%\fs`:

- the certificate (`cert.pem`, `key.pem`)
- the default folder (`default-path`)
- one record and one log per service, in `services\`

Logins shown by `fs ps` are stored there too, readable only by your Windows
user.

## Troubleshooting

- **"port 80 is already in use"**: another program, such as IIS, another web
  server or another fs service (see `fs ps`), is using it. Stop it, or choose
  another port: `--port 8080`.
- **"access denied"**: the port may be reserved by Windows. Use another port.
- **Other devices can't connect**: allow fs through the firewall when Windows
  asks (Private networks), and use the "Network" address fs prints.
- **A background service won't start**: `fs start` shows the error. Run
  `fs logs <id>` for the full log.
