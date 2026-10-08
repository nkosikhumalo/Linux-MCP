<div align="center">

# 🐹 Ubuntu Dev Assistant

### Your Linux machine, explained in plain English.

[![Built with Go](https://img.shields.io/badge/Built%20with-Go-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)
[![Desktop app](https://img.shields.io/badge/Desktop-Wails-DF0000?style=for-the-badge&logo=wails&logoColor=white)](https://wails.io/)
[![AI powered](https://img.shields.io/badge/AI-OpenRouter-7C3AED?style=for-the-badge)](https://openrouter.ai/)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20Ubuntu-E95420?style=for-the-badge&logo=ubuntu&logoColor=white)](https://ubuntu.com/)

**Ask about your computer. Get useful answers. Let the assistant handle everyday tasks.**

</div>

---

## What is it?

Ubuntu Dev Assistant is a desktop chat app and a standalone [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server for Linux. You can ask an AI questions about your machine in everyday language. The assistant can look up system details with local tools, explain what it finds, and help with common actions such as opening an app.

The app is written in **Go**. Its desktop window is built with **Wails**, and its chat model can run through a loopback local model endpoint by default, or through **OpenRouter** when explicitly enabled. The MCP server makes the same machine tools available to other MCP-compatible AI apps.

> Think of it as a helpful guide to your computer: ask “What is using port 8080?” or “How much disk space do I have?” and it checks the relevant local information for you.

## At a glance

| | What it does |
| --- | --- |
| | Chat with your computer using simple prompts |
| | Inspect system information, processes, ports, disk, and memory |
| | Find and open installed apps, including Snap, Flatpak, and PWAs |
| | Use the same tools from an MCP-compatible client |
| | Save, reopen, delete, and download local chat history; use local inference or explicitly opt into OpenRouter |
| | Switch between dark and light themes in the desktop app |

## What can it do?

The built-in tools can:

- Show your Linux distribution and kernel details.
- Check free disk space, directory sizes, the largest files in a folder, and RAM usage.
- Find processes, show top CPU or memory users, and inspect listening TCP/UDP ports.
- Search for apps, view available APT/DEB install history, and open or close supported apps.
- Open a URL or file with your default desktop handler.
- Read selected system journal entries and check Git status or recent commits.
- Run Go tests in a project directory.
- Run a read-only ClamAV scan, view detections live, and stop it from the desktop app when needed.
- Stop a user-owned process by PID or exact process name.

The AI can request a tool, but state-changing or code-executing tools wait for your approval in the desktop app. The standalone MCP server denies those tools because it has no approval UI. The AI decides when to call read-only tools and then explains the result. Some tools depend on Linux utilities being installed, and some results depend on your account's permissions. The security scan does not use `sudo`.

Storage audits inspect and report only. The largest-file scan uses a bounded min-heap, optional extension hash filtering, a traversal entry limit, and skips virtual directories, symlinks, and mounted filesystems on another device. It reports the largest files; use the folder-size tool to compare directory totals and find caches or other storage-heavy folders. In the desktop app, each reported file has a “Show in Files” link that selects it when the file manager supports selection. Audits do not determine whether files or installed applications are unused, and do not delete or move anything; users decide whether to remove anything themselves. APT/DEB installation dates come from local package logs, which may be incomplete and can include dependencies rather than user-facing apps. Snap, Flatpak, PWA, and manually installed app dates are reported as unknown when no reliable local date is available.

## How it fits together

```text
You ──► Wails desktop chat ──► local loopback model (default) / OpenRouter (opt-in)
                                  │
                                  └── requests a local tool
                                           │
                                           ▼
                                  Go system tools ──► Linux

MCP-compatible client ──► stdio MCP server ──► same Go system tools
```

In local mode, the desktop chat sends your message and available tool descriptions only to the configured loopback model endpoint. In OpenRouter mode, it sends them to the selected OpenRouter model. When the model asks for a system check, the Go app runs the matching local tool and sends its result back so the model can answer. The separate MCP server speaks MCP over standard input and output; it does not start the desktop window or call the chat model.

## Get started

### You will need

- Linux (the built-in system tools target Ubuntu and similar Linux desktops).
- Go **1.25.5** or newer, matching the version in `go.mod`.
- The [Wails v2 CLI](https://wails.io/docs/gettingstarted/installation/) and its Linux desktop build dependencies to run or build the GUI.
- A local model server with an OpenAI-compatible API endpoint for the default private mode, or an [OpenRouter API key](https://openrouter.ai/keys) if you explicitly choose cloud mode. The standalone MCP server does not need an API key.

### 1. Get the project

```bash
git clone <your-repository-url>
cd ubuntu-dev-assistant
```

Replace `<your-repository-url>` with the URL of your Git repository.

### 2. Configure the model

Copy the example settings:

```bash
cp .env.example .env
```

Local inference is the default for new configurations. Set `LOCAL_MODEL_BASE_URL` to your local model server and `LOCAL_MODEL_NAME` to a model it provides. The endpoint must use HTTP on the loopback IP `127.0.0.1` or `::1`; the app rejects hostnames and remote endpoints, disables environment proxy use, and does not follow redirects in local mode.

To use OpenRouter in a new configuration, explicitly set `AI_MODE=openrouter` and provide `OPENROUTER_API_KEY`. Existing setups with an OpenRouter key and no `AI_MODE` keep using OpenRouter for compatibility; set `AI_MODE=local` to prevent cloud fallback. In that mode, your chat, conversation context, and tool results are sent to the cloud provider; check its retention and training policy. Keep `.env` private and out of Git.

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

### 4. Run the MCP server (optional)

Start the standalone server from the project root:

```bash
go run ./cmd/mcp-server
```

It communicates over **stdio**, so it is intended to be launched by an MCP client. For a client that supports a JSON server configuration, the entry usually looks like this (use the full path to your project):

```json
{
  "mcpServers": {
    "ubuntu-dev-assistant": {
      "command": "go",
      "args": ["run", "./cmd/mcp-server"],
      "cwd": "/absolute/path/to/ubuntu-dev-assistant"
    }
  }
}
```

Client configuration formats differ; check your MCP client's docs for the correct place to add this entry. Keep standard output reserved for MCP messages when launching the server through a client.

## Settings

The app reads these optional settings from the environment or `.env` file:

| Setting | Default | Description |
| --- | --- | --- |
| `AI_MODE` | From configured settings | `local` keeps model requests on loopback; `openrouter` selects OpenRouter. If unset, a complete local configuration is preferred, otherwise an existing OpenRouter key preserves the previous setup |
| `LOCAL_MODEL_BASE_URL` | *(required in local mode)* | HTTP endpoint on a loopback IP, such as `http://127.0.0.1:11434/v1` |
| `LOCAL_MODEL_NAME` | *(required in local mode)* | Model name served by the local endpoint |
| `OPENROUTER_API_KEY` | *(required in OpenRouter mode)* | Your OpenRouter API key |
| `OPENROUTER_FAST_MODEL` | `openai/gpt-4o-mini` | Model used for ordinary questions |
| `OPENROUTER_HEAVY_MODEL` | `openai/gpt-4o-mini` | Model used for longer or more involved system questions |
| `OPENROUTER_MAX_TOKENS` | `1024` | Maximum response tokens requested from the model |

In OpenRouter mode, the fast and heavy model names can be changed to models available through your OpenRouter account. The router picks between them using simple message hints; by default both settings point to the same model.

## Local chat history

Desktop conversations are saved as individual JSON files under `~/.local/share/ubuntu-dev-assistant/conversations/` with user-only file permissions. There is no database or cloud history store in this feature. The app can reopen a saved chat, continue it with recent messages as model context, delete it, or export a JSON copy to a location chosen by the user. Saved history contains visible user and assistant messages; internal tool traces and tool results are not saved. Continuing a chat sends its recent context to the configured model endpoint as part of the ordinary chat request. With local mode, that endpoint is restricted to loopback; with OpenRouter mode, the context leaves the machine.

## Built-in MCP tools

| Tool | In simple words |
| --- | --- |
| `os_info` | Identify the operating system and kernel |
| `disk_free`, `disk_usage` | Check disk space and folder sizes |
| `storage_audit` | Find the largest files under a folder with optional filename or extension filtering and bounded memory (read-only; filename searches ignore size thresholds and scan the full accessible scope) |
| `mem_free` | Check memory (RAM) use |
| `top_processes`, `find_process` | Find active processes and heavy CPU/RAM users |
| `inspect_port`, `list_listening_ports` | See which processes are listening on network ports |
| `list_apps`, `open_app`, `close_app` | Find, launch, and close supported desktop apps |
| `app_install_history` | Read dated APT/DEB package events from available local logs |
| `open_uri` | Open a URL or local file with its default app |
| `stop_process` | Ask a user-owned process to stop |
| `security_scan` | Scan a folder with ClamAV, if installed |
| `journalctl` | Read filtered systemd journal entries |
| `git_status`, `git_log` | View a repository's status and recent commits |
| `go_test` | Run Go tests in a project directory |

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
- The MCP server exposes local machine tools to whichever MCP client launches it; that client can see returned data and may send it to its own model provider. Only add it to a client you trust. State-changing and code-executing tools are denied by the standalone server because it cannot ask you for approval.

## Built with

[![Go](https://img.shields.io/badge/Go-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![Wails](https://img.shields.io/badge/Wails-DF0000?style=flat-square&logo=wails&logoColor=white)](https://wails.io/)
[![OpenRouter](https://img.shields.io/badge/OpenRouter-7C3AED?style=flat-square)](https://openrouter.ai/)
[![MCP](https://img.shields.io/badge/Model%20Context%20Protocol-MCP-4B5563?style=flat-square)](https://modelcontextprotocol.io/)

---

<div align="center">

**Made for curious people who want to understand their machine.**

</div>
