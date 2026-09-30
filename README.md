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

## ✨ What is it?

Ubuntu Dev Assistant is a desktop chat app and a standalone [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server for Linux. You can ask an AI questions about your machine in everyday language. The assistant can look up system details with local tools, explain what it finds, and help with common actions such as opening an app.

The app is written in **Go**. Its desktop window is built with **Wails**, and its chat model is provided through **OpenRouter**. The MCP server makes the same machine tools available to other MCP-compatible AI apps.

> 🛠️ Think of it as a helpful guide to your computer: ask “What is using port 8080?” or “How much disk space do I have?” and it checks the relevant local information for you.

## 🎨 At a glance

| | What it does |
| --- | --- |
| 💬 | Chat with your computer using simple prompts |
| 🔎 | Inspect system information, processes, ports, disk, and memory |
| 📦 | Find and open installed apps, including Snap, Flatpak, and PWAs |
| 🧰 | Use the same tools from an MCP-compatible client |
| 🧠 | Keep a short conversation history and choose a model through OpenRouter |
| 🌗 | Switch between dark and light themes in the desktop app |

## 🧩 What can it do?

The built-in tools can:

- Show your Linux distribution and kernel details.
- Check free disk space, directory sizes, and RAM usage.
- Find processes, show top CPU or memory users, and inspect listening TCP/UDP ports.
- Search for apps and open or close supported apps.
- Open a URL or file with your default desktop handler.
- Read selected system journal entries and check Git status or recent commits.
- Run Go tests in a project directory.
- Run a basic ClamAV scan when ClamAV is installed.
- Stop a user-owned process by PID or exact process name.

The AI decides when to call a tool and then explains the result. Some tools depend on Linux utilities being installed, and some results depend on your account's permissions. The security scan does not use `sudo`.

## 🏗️ How it fits together

```text
You ──► Wails desktop chat ──► OpenRouter model
                                  │
                                  └── requests a local tool
                                           │
                                           ▼
                                  Go system tools ──► Linux

MCP-compatible client ──► stdio MCP server ──► same Go system tools
```

The desktop chat sends your message and available tool descriptions to the selected OpenRouter model. When the model asks for a system check, the Go app runs the matching local tool and sends its result back so the model can answer. The separate MCP server speaks MCP over standard input and output; it does not start the desktop window or call the chat model.

## 🚀 Get started

### You will need

- Linux (the built-in system tools target Ubuntu and similar Linux desktops).
- Go **1.25.5** or newer, matching the version in `go.mod`.
- The [Wails v2 CLI](https://wails.io/docs/gettingstarted/installation/) and its Linux desktop build dependencies to run or build the GUI.
- An [OpenRouter API key](https://openrouter.ai/keys) for the desktop chat. The standalone MCP server does not need an API key.

### 1. Get the project

```bash
git clone <your-repository-url>
cd ubuntu-dev-assistant
```

Replace `<your-repository-url>` with the URL of your Git repository.

### 2. Add your OpenRouter key

Copy the example settings and add your key:

```bash
cp .env.example .env
```

Open `.env` and set `OPENROUTER_API_KEY`:

```dotenv
OPENROUTER_API_KEY=your_openrouter_api_key
```

Keep your key private. `.env` is excluded from Git, so it should stay on your machine.

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

## ⚙️ Settings

The app reads these optional settings from the environment or `.env` file:

| Setting | Default | Description |
| --- | --- | --- |
| `OPENROUTER_API_KEY` | *(required for chat)* | Your OpenRouter API key |
| `OPENROUTER_FAST_MODEL` | `openai/gpt-4o-mini` | Model used for ordinary questions |
| `OPENROUTER_HEAVY_MODEL` | `openai/gpt-4o-mini` | Model used for longer or more involved system questions |
| `OPENROUTER_MAX_TOKENS` | `1024` | Maximum response tokens requested from the model |

The fast and heavy model names can be changed to models available through your OpenRouter account. The router picks between them using simple message hints; by default both settings point to the same model.

## 🧰 Built-in MCP tools

| Tool | In simple words |
| --- | --- |
| `os_info` | Identify the operating system and kernel |
| `disk_free`, `disk_usage` | Check disk space and folder sizes |
| `mem_free` | Check memory (RAM) use |
| `top_processes`, `find_process` | Find active processes and heavy CPU/RAM users |
| `inspect_port`, `list_listening_ports` | See which processes are listening on network ports |
| `list_apps`, `open_app`, `close_app` | Find, launch, and close supported desktop apps |
| `open_uri` | Open a URL or local file with its default app |
| `stop_process` | Ask a user-owned process to stop |
| `security_scan` | Scan a folder with ClamAV, if installed |
| `journalctl` | Read filtered systemd journal entries |
| `git_status`, `git_log` | View a repository's status and recent commits |
| `go_test` | Run Go tests in a project directory |

## 🗂️ Project map

```text
.
├── app.go                 # Connects the desktop interface to Go code
├── main.go                # Starts the Wails desktop app
├── cmd/mcp-server/        # Standalone stdio MCP server
├── mcp/                   # Local Linux tools and MCP tool registry
├── pipeline/              # OpenRouter client and chat/tool routing
└── frontend/              # Desktop interface (HTML, CSS, and JavaScript)
```

## 🔐 A few practical notes

- The AI chat needs an internet connection and sends your prompt, conversation context, and any tool results used for that reply to OpenRouter. Avoid including passwords, API keys, or other secrets in chat.
- System tools run on your machine under your user account. They do not automatically gain administrator access, and they cannot read files your account cannot access.
- App discovery and launch work with desktop entries and common Ubuntu formats; exact support depends on what is installed on your system.
- The MCP server exposes local machine tools to whichever MCP client launches it. Only add it to a client you trust.

## 💙 Built with

[![Go](https://img.shields.io/badge/Go-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![Wails](https://img.shields.io/badge/Wails-DF0000?style=flat-square&logo=wails&logoColor=white)](https://wails.io/)
[![OpenRouter](https://img.shields.io/badge/OpenRouter-7C3AED?style=flat-square)](https://openrouter.ai/)
[![MCP](https://img.shields.io/badge/Model%20Context%20Protocol-MCP-4B5563?style=flat-square)](https://modelcontextprotocol.io/)

---

<div align="center">

**Made for curious people who want to understand their machine.** 🐧

</div>
