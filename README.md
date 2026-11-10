<div align="center">

# Linux Helper

### Understand your Linux machine. Get real answers from the tools on it.

[![Built with Go](https://img.shields.io/badge/Built%20with-Go-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)
[![Desktop app](https://img.shields.io/badge/Desktop-Wails-DF0000?style=for-the-badge&logo=wails&logoColor=white)](https://wails.io/)
[![Models](https://img.shields.io/badge/Models-Local%20%2B%20OpenRouter-87CEFA?style=for-the-badge)](https://openrouter.ai/)
[![Platform](https://img.shields.io/badge/Platform-Linux-111111?style=for-the-badge&logo=linux&logoColor=white)](https://kernel.org/)

**Ask a question. Check the evidence. Choose before anything changes.**

</div>

---

## What is it?

Linux Helper is a desktop chat app and a standalone [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server for Linux. Ask questions about your machine in everyday language. The assistant can check system details with local tools, explain what it finds, and help with common actions such as opening an app.

The app is written in **Go**. Its desktop window is built with **Wails**, and its chat model can run through a loopback local model endpoint by default, or through **OpenRouter** when explicitly enabled. The MCP server makes the same machine tools available to other MCP-compatible AI apps.

> Think of it as a helpful guide to your computer: ask “What is using port 8080?” or “How much disk space do I have?” and it checks the relevant local information for you.

## At a glance

| Feature | What it does |
| --- | --- |
| Natural-language system help | Ask about Linux, processes, ports, storage, memory, apps, and logs |
| Evidence-based investigations | Gather relevant read-only results over multiple bounded rounds and summarize what the tools found |
| ClamAV scans | View scan findings and scan state in the desktop chat; stop a scan from the app |
| Command proposals | Review and copy the exact command; it runs only after you press **Execute** |
| Local chat history | Start fresh, reopen, export, or delete conversations saved on this device |
| Local or cloud models | Keep requests on a configured loopback model, or opt into OpenRouter's free-model router |
| Desktop chat | A focused Wails interface with dark and light themes |
| MCP server | Make the same system tools available to a compatible MCP client |

## What can it do?

The built-in tools can:

- Show your Linux distribution and kernel details.
- Check free disk space, directory sizes, the largest files in a folder, and RAM usage.
- Find processes, show top CPU or memory users, and inspect listening TCP/UDP ports.
- Search for apps, view available APT/DEB install history, open or close supported apps, and uninstall APT/Snap/Flatpak apps after explicit approval. Manual installs and browser apps are reported as unsupported for automatic removal.
- Open a URL or file with your default desktop handler.
- Read selected system journal entries and check Git status or recent commits.
- Run Go tests in a project directory.
- Propose a shell command, show the exact command for review, and run it only after you press **Execute** in the desktop app.
- Run a read-only ClamAV scan, view detections live, and stop it from the desktop app when needed.
- Stop a user-owned process by PID or exact process name.

Read-only checks can run when needed to answer a question. State-changing or code-executing tools—including shell commands, app removal, process stopping, and Go tests—wait for explicit approval in the desktop app. A command card shows the exact command and offers **Copy**, **Execute**, and **Cancel**. Approved commands run for up to two minutes and can be stopped from the app. Commands that need administrator authentication are not given a password prompt in chat; the assistant directs you to run them in a terminal where you can enter your password.

The standalone MCP server has no approval UI, so it denies state-changing tools. To enable only app removal in a trusted MCP client, set `MCP_ALLOW_APP_REMOVAL=true` in that server's environment; the MCP client must provide its own confirmation. Some tools depend on Linux utilities being installed, and results depend on your account's permissions. The security scan does not use `sudo`.

For deep investigations and scan requests, the desktop assistant can gather relevant read-only evidence over up to 12 model/tool rounds, skip identical repeated queries, and synthesize the findings after requested tools return. The scan card reports real ClamAV start, finding, and completion events; the investigation indicator is indeterminate while a scan is running because ClamAV does not provide a reliable percentage. A stopped, timed-out, or partial scan is not reported as clean.

Storage audits inspect and report only. The largest-file scan ranks by filesystem-reported allocated space, also shows logical file length, and lists detected hard-linked files once. Sparse files can have a much smaller allocated size than logical length. The scan uses a bounded min-heap, optional extension hash filtering, a traversal entry limit, and skips virtual directories, symlinks, and mounted filesystems on another device. Use the folder-size tool to compare directory totals and find caches or other storage-heavy folders. In the desktop app, each reported file has a “Show in Files” link that selects it when the file manager supports selection. Audits do not determine whether files or installed applications are unused, and do not delete or move anything; users decide whether to remove anything themselves. APT/DEB installation dates come from local package logs, which may be incomplete and can include dependencies rather than user-facing apps. Snap, Flatpak, PWA, and manually installed app dates are reported as unknown when no reliable local date is available.

## How it fits together

```text
You ──► Wails desktop chat ──► local loopback model (default) / OpenRouter (free router, opt-in)
                                  │
                                  └── requests a local tool
                                           │
                                           ▼
                                  Go system tools ──► Linux

MCP-compatible client ──► stdio MCP server ──► same Go system tools
```

In local mode, the desktop chat sends your message and available tool descriptions only to the configured loopback model endpoint. In OpenRouter mode, it sends them to the selected model; the default `openrouter/free` router selects an available free model. Free models can be rate-limited or temporarily unavailable. When the model asks for a system check, the Go app runs the matching local tool and sends its result back so the model can answer. The separate MCP server speaks MCP over standard input and output; it does not start the desktop window or call the chat model.

## Get started

### You will need

- Linux (the built-in system tools target Ubuntu and similar Linux desktops).
- Go **1.25.5** or newer, matching the version in `go.mod`.
- The [Wails v2 CLI](https://wails.io/docs/gettingstarted/installation/) and its Linux desktop build dependencies to run or build the GUI.
- A local model server with an OpenAI-compatible API endpoint for private local mode, or an [OpenRouter API key](https://openrouter.ai/keys) if you choose cloud mode. OpenRouter's free-model router does not require purchased model credits, but still has provider availability and rate limits. The standalone MCP server does not need an API key.

### 1. Get the project

```bash
git clone <your-repository-url>
cd linux-helper
```

Replace `<your-repository-url>` with the URL of your Git repository.

### 2. Configure the model

Copy the example settings:

```bash
cp .env.example .env
```

Local inference is the default for new configurations. Set `LOCAL_MODEL_BASE_URL` to your local model server and `LOCAL_MODEL_NAME` to a model it provides. The endpoint must use HTTP on the loopback IP `127.0.0.1` or `::1`; the app rejects hostnames and remote endpoints, disables environment proxy use, and does not follow redirects in local mode.

To use OpenRouter, set `AI_MODE=openrouter` and provide `OPENROUTER_API_KEY`. The example configuration selects `openrouter/free` for both ordinary chat and deeper investigations. Existing setups with an OpenRouter key and no `AI_MODE` keep using OpenRouter for compatibility; set `AI_MODE=local` to prevent cloud fallback. In cloud mode, your chat, conversation context, and tool results are sent to the cloud provider; check its retention and training policy. Keep `.env` private and out of Git.

### 3. Launch the desktop app

For development:

```bash
wails dev
```

To build a desktop application:

```bash
wails build
```

Wails may need Linux development libraries for GTK and WebKitGTK. See the [Wails Linux setup guide](https://wails.io/docs/gettingstarted/installation/) for the package list for your distribution.

## Linux compatibility

This project targets Linux desktops, but it is not guaranteed to run on every Linux distribution or machine. The Wails build is configured with the `webkit2_41` tag, so the desktop app needs GTK and WebKit2GTK **4.1** runtime libraries. Compatible examples include Ubuntu 22.04 or newer, Debian 12 or newer, Fedora 40 or newer, Arch Linux, and openSUSE; package names and installation steps vary. See the [Wails v2 distro guide](https://v2.wails.io/docs/v2.15.0/guides/linux-distro-support/) before building or packaging for another system.

Most system checks use common Linux utilities such as `free`, `ss`, `df`, `du`, and `ps`. Some features need additional software or a specific system setup:

- App install history and APT package removal use `dpkg`/APT, so they are intended for Debian-based systems.
- Snap and Flatpak actions need their respective package managers to be installed.
- Journal queries need `journalctl` and a systemd-based system.
- Virus scans need ClamAV's `clamscan` utility.
- Opening files and desktop apps depends on desktop entries and `xdg-open` being available.

On other distributions, features may be unavailable when their required tool or package format is absent. The assistant reports those limitations; distribution-wide compatibility has not been tested.

### 4. Run the MCP server (optional)

Start the standalone server from the project root:

```bash
go run ./cmd/mcp-server
```

It communicates over **stdio**, so it is intended to be launched by an MCP client. For a client that supports a JSON server configuration, the entry usually looks like this (use the full path to your project):

```json
{
  "mcpServers": {
    "linux-helper": {
      "command": "go",
      "args": ["run", "./cmd/mcp-server"],
      "cwd": "/absolute/path/to/linux-helper"
    }
  }
}
```

Client configuration formats differ; check your MCP client's docs for the correct place to add this entry. Keep standard output reserved for MCP messages when launching the server through a client. To allow app removal through this stdio server, set `MCP_ALLOW_APP_REMOVAL=true` in the server process environment using your MCP client's configuration. Without it, app removal stays disabled because stdio has no built-in confirmation dialog.

## Settings

The app reads these optional settings from the environment or `.env` file:

| Setting | Default | Description |
| --- | --- | --- |
| `AI_MODE` | From configured settings | `local` keeps model requests on loopback; `openrouter` selects OpenRouter. If unset, a complete local configuration is preferred, otherwise an existing OpenRouter key preserves the previous setup |
| `LOCAL_MODEL_BASE_URL` | *(required in local mode)* | HTTP endpoint on a loopback IP, such as `http://127.0.0.1:11434/v1` |
| `LOCAL_MODEL_NAME` | *(required in local mode)* | Model name served by the local endpoint |
| `OPENROUTER_API_KEY` | *(required in OpenRouter mode)* | Your OpenRouter API key |
| `OPENROUTER_FAST_MODEL` | `openrouter/free` | Free model router used for ordinary questions |
| `OPENROUTER_HEAVY_MODEL` | `openrouter/free` | Free model router used for longer or more involved system questions |
| `OPENROUTER_MAX_TOKENS` | `1024` (fallback) | Maximum response tokens requested from the model; `.env.example` sets this to `2048` |

Model requests can take up to ten minutes; use **Stop** to cancel a stalled request. This limit applies per model request, not to the total duration of a multi-round investigation.

In OpenRouter mode, the fast and heavy model names can be changed to models available through your OpenRouter account. The router picks between them using simple message hints; by default both settings point to the same model.

## Local chat history

Desktop conversations are saved as individual JSON files under `~/.local/share/ubuntu-dev-assistant/conversations/` with user-only file permissions. This existing data directory name is retained across the product rename so saved chats remain available. There is no database or cloud history store in this feature. Every app launch opens a new chat; saved chats remain available in the history drawer. You can reopen, continue, delete, or export a JSON copy of a conversation. Saved history contains visible user and assistant messages; internal tool traces and tool results are not saved. Continuing a chat sends its recent context to the configured model endpoint as part of the ordinary chat request. With local mode, that endpoint is restricted to loopback; with OpenRouter mode, the context leaves the machine.

## Built-in MCP tools

| Tool | In simple words |
| --- | --- |
| `os_info` | Identify the operating system and kernel |
| `disk_free`, `disk_usage` | Check disk space and folder sizes |
| `storage_audit` | Find the largest files under a folder with optional filename or extension filtering and bounded memory (read-only; filename searches ignore size thresholds and scan the full accessible scope) |
| `mem_free` | Check memory (RAM) use |
| `top_processes`, `find_process` | Find active processes and heavy CPU/RAM users |
| `inspect_port`, `list_listening_ports` | See which processes are listening on network ports |
| `list_apps`, `open_app`, `close_app`, `remove_app` | Find, launch, close, and uninstall supported apps; uninstall requires approval |
| `app_install_history` | Read dated APT/DEB package events from available local logs |
| `open_uri` | Open a URL or local file with its default app |
| `stop_process` | Ask a user-owned process to stop |
| `security_scan` | Scan a folder with ClamAV, if installed |
| `journalctl` | Read filtered systemd journal entries |
| `git_status`, `git_log` | View a repository's status and recent commits |
| `go_test` | Run Go tests in a project directory |
| `run_command` | Propose a shell command; desktop execution requires an explicit **Execute** click and is unavailable in the standalone MCP server |

## Project map

```text
.
├── app.go                 # Connects the desktop interface to Go code
├── main.go                # Starts the Wails desktop app
├── cmd/mcp-server/        # Standalone stdio MCP server
├── mcp/                   # Local Linux tools and MCP tool registry
├── pipeline/              # Local/OpenRouter clients and chat/tool routing
└── frontend/              # Desktop interface (HTML, CSS, and JavaScript)
```

## A few practical notes

- Local mode does not send prompts or tool results to a cloud model, but the local model server is software running on your machine and should be kept up to date. OpenRouter mode sends prompts, conversation context, and tool results to OpenRouter and its selected provider; avoid secrets and review that provider's data policy.
- System tools run on your machine under your user account. They do not automatically gain administrator access, and they cannot read files your account cannot access.
- App discovery and launch work with desktop entries and common Ubuntu formats; exact support depends on what is installed on your system.
- The MCP server exposes local machine tools to whichever MCP client launches it; that client can see returned data and may send it to its own model provider. Only add it to a client you trust. State-changing tools stay disabled by default in the stdio server. App removal can be enabled with `MCP_ALLOW_APP_REMOVAL=true`; other state-changing tools still require an approval UI.

## Built with

[![Go](https://img.shields.io/badge/Go-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![Wails](https://img.shields.io/badge/Wails-DF0000?style=flat-square&logo=wails&logoColor=white)](https://wails.io/)
[![OpenRouter](https://img.shields.io/badge/OpenRouter-7C3AED?style=flat-square)](https://openrouter.ai/)
[![MCP](https://img.shields.io/badge/Model%20Context%20Protocol-MCP-4B5563?style=flat-square)](https://modelcontextprotocol.io/)

---

<div align="center">

**Made for curious people who want to understand their machine.**

</div>
