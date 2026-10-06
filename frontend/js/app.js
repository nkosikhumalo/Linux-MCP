(() => {
  const thread = document.getElementById("thread");
  const input = document.getElementById("input");
  const sendBtn = document.getElementById("send");
  const themeBtn = document.getElementById("theme-btn");
  const themeLabel = document.getElementById("theme-label");
  const newChatBtn = document.getElementById("new-chat");
  const historyToggle = document.getElementById("history-toggle");
  const historyPanel = document.getElementById("history-panel");
  const historyList = document.getElementById("history-list");
  const workspace = document.querySelector(".workspace");
  let activeConversationId = null;
  let currentMessages = [];
  let historyReady = false;

  const THEME_KEY = "uda-theme";

  function applyTheme(theme) {
    const next = theme === "light" ? "light" : "dark";
    document.documentElement.setAttribute("data-theme", next);
    themeLabel.textContent = next === "dark" ? "Light" : "Dark";
    localStorage.setItem(THEME_KEY, next);
  }

  applyTheme(localStorage.getItem(THEME_KEY) || "dark");

  themeBtn.addEventListener("click", () => {
    const cur = document.documentElement.getAttribute("data-theme");
    applyTheme(cur === "dark" ? "light" : "dark");
  });

  function resizeInput() {
    input.style.height = "auto";
    input.style.height = Math.min(input.scrollHeight, 128) + "px";
  }

  function syncSend() {
    sendBtn.disabled = !input.value.trim() || sendBtn.dataset.busy === "1" || !historyReady;
  }

  input.addEventListener("input", () => {
    resizeInput();
    syncSend();
  });

  input.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      if (!sendBtn.disabled) send();
    }
  });

  sendBtn.addEventListener("click", send);
  newChatBtn.addEventListener("click", startNewConversation);
  historyToggle.addEventListener("click", toggleHistory);

  function el(tag, className, text) {
    const node = document.createElement(tag);
    if (className) node.className = className;
    if (text != null) node.textContent = text;
    return node;
  }

  function escapeHtml(s) {
    return String(s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  // Small Markdown subset for chat: headings, lists, bold/italic, code, links, paragraphs.
  function renderMarkdown(src) {
    const escaped = escapeHtml(src).replace(/\r\n/g, "\n");
    const lines = escaped.split("\n");
    const out = [];
    let inUl = false;
    let inOl = false;
    let inCode = false;
    let codeBuf = [];

    const closeLists = () => {
      if (inUl) {
        out.push("</ul>");
        inUl = false;
      }
      if (inOl) {
        out.push("</ol>");
        inOl = false;
      }
    };

    const inline = (t) =>
      t
        .replace(/`([^`]+)`/g, "<code>$1</code>")
        .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")
        .replace(/(^|[^*\w])\*([^*\n]+)\*(?!\*)/g, "$1<em>$2</em>")
        .replace(
          /\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g,
          '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>'
        );

    for (const line of lines) {
      if (line.startsWith("```")) {
        if (inCode) {
          out.push("<pre><code>" + codeBuf.join("\n") + "</code></pre>");
          codeBuf = [];
          inCode = false;
        } else {
          closeLists();
          inCode = true;
        }
        continue;
      }
      if (inCode) {
        codeBuf.push(line);
        continue;
      }

      const h = /^(#{1,3})\s+(.+)$/.exec(line);
      if (h) {
        closeLists();
        const n = h[1].length;
        out.push(`<h${n}>${inline(h[2])}</h${n}>`);
        continue;
      }

      const ul = /^[-*]\s+(.+)$/.exec(line);
      if (ul) {
        if (inOl) {
          out.push("</ol>");
          inOl = false;
        }
        if (!inUl) {
          out.push("<ul>");
          inUl = true;
        }
        out.push(`<li>${inline(ul[1])}</li>`);
        continue;
      }

      const ol = /^(\d+)\.\s+(.+)$/.exec(line);
      if (ol) {
        if (inUl) {
          out.push("</ul>");
          inUl = false;
        }
        if (!inOl) {
          out.push("<ol>");
          inOl = true;
        }
        out.push(`<li>${inline(ol[2])}</li>`);
        continue;
      }

      if (line.trim() === "") {
        closeLists();
        continue;
      }

      closeLists();
      out.push(`<p>${inline(line)}</p>`);
    }

    if (inCode) out.push("<pre><code>" + codeBuf.join("\n") + "</code></pre>");
    closeLists();
    return out.join("");
  }

  function appendMessage(role, text, opts = {}) {
    const wrap = el("article", `msg ${role}${opts.error ? " error" : ""}${opts.pending ? " pending" : ""}`);
    wrap.appendChild(el("div", "msg-meta", role === "user" ? "You" : "Assistant"));
    const body = el("div", "msg-body");
    if (role === "assistant" && !opts.pending && !opts.error) {
      body.classList.add("md");
      body.innerHTML = renderMarkdown(text);
    } else {
      body.textContent = text;
    }
    wrap.appendChild(body);
    // Trace stays in the backend only — not shown in the chat UI.
    thread.appendChild(wrap);
    thread.scrollTop = thread.scrollHeight;
    return wrap;
  }

  function appBridge() {
    return window.go && window.go.main && window.go.main.App;
  }

  async function appCall(method, ...args) {
    const bridge = appBridge();
    if (!bridge || typeof bridge[method] !== "function") {
      throw new Error("Backend not connected. Run this UI through Wails (wails dev).");
    }
    return bridge[method](...args);
  }

  function field(obj, lower, upper) {
    return obj && (obj[lower] !== undefined ? obj[lower] : obj[upper]);
  }

  async function startNewConversation() {
    if (sendBtn.dataset.busy === "1") return;
    try {
      const summary = await appCall("NewConversation");
      activeConversationId = field(summary, "id", "ID");
      currentMessages = [];
      thread.replaceChildren();
      await renderHistory();
      input.focus();
    } catch (err) {
      appendMessage("assistant", err.message || String(err), { error: true });
    }
  }

  async function openConversation(id) {
    if (sendBtn.dataset.busy === "1") return;
    try {
      const conv = await appCall("OpenConversation", id);
      activeConversationId = field(conv, "id", "ID");
      currentMessages = field(conv, "messages", "Messages") || [];
      thread.replaceChildren();
      for (const msg of currentMessages) {
        appendMessage(field(msg, "role", "Role"), field(msg, "content", "Content"));
      }
      await renderHistory();
      if (window.matchMedia("(max-width: 560px)").matches) historyPanel.classList.remove("mobile-open");
      input.focus();
    } catch (err) {
      appendMessage("assistant", err.message || String(err), { error: true });
    }
  }

  async function renderHistory() {
    const summaries = await appCall("ListConversations");
    historyList.replaceChildren();
    if (!summaries || summaries.length === 0) {
      historyList.appendChild(el("div", "history-empty", "Your saved chats will show up here."));
      return;
    }
    for (const item of summaries) {
      const id = field(item, "id", "ID");
      const title = field(item, "title", "Title") || "Untitled chat";
      const updated = field(item, "updatedAt", "UpdatedAt");
      const row = el("div", `history-row${id === activeConversationId ? " active" : ""}`);
      const open = el("button", "history-open");
      open.type = "button";
      open.title = title;
      open.appendChild(el("span", "history-title", title));
      open.appendChild(el("span", "history-date", updated ? new Date(updated).toLocaleString() : ""));
      open.addEventListener("click", () => openConversation(id));
      const exportBtn = el("button", "history-action", "Export");
      exportBtn.type = "button";
      exportBtn.title = "Download this conversation";
      exportBtn.addEventListener("click", () => downloadConversation(id));
      const deleteBtn = el("button", "history-action delete", "Delete");
      deleteBtn.type = "button";
      deleteBtn.title = "Delete this conversation";
      deleteBtn.addEventListener("click", () => deleteConversation(id, title));
      row.append(open, exportBtn, deleteBtn);
      historyList.appendChild(row);
    }
  }

  async function deleteConversation(id, title) {
    if (!window.confirm(`Delete “${title}” from this device?`)) return;
    try {
      await appCall("DeleteConversation", id);
      if (id === activeConversationId) {
        activeConversationId = null;
        currentMessages = [];
        thread.replaceChildren();
        const summary = await appCall("NewConversation");
        activeConversationId = field(summary, "id", "ID");
      }
      await renderHistory();
    } catch (err) {
      appendMessage("assistant", err.message || String(err), { error: true });
    }
  }

  async function downloadConversation(id) {
    try {
      const conv = await appCall("GetConversation", id);
      const title = field(conv, "title", "Title") || "conversation";
      const messages = field(conv, "messages", "Messages") || [];
      const content = `# ${title}\n\n` + messages.map((msg) => {
        const role = field(msg, "role", "Role") === "user" ? "You" : "Assistant";
        return `## ${role}\n\n${field(msg, "content", "Content") || ""}`;
      }).join("\n\n---\n\n") + "\n";
      const blob = new Blob([content], { type: "text/markdown;charset=utf-8" });
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = `${title.replace(/[^a-z0-9_-]+/gi, "-").replace(/^-|-$/g, "") || "conversation"}.md`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
    } catch (err) {
      appendMessage("assistant", err.message || String(err), { error: true });
    }
  }

  function toggleHistory() {
    if (window.matchMedia("(max-width: 560px)").matches) {
      historyPanel.classList.toggle("mobile-open");
      historyToggle.setAttribute("aria-expanded", historyPanel.classList.contains("mobile-open") ? "true" : "false");
      return;
    }
    const hidden = historyPanel.classList.toggle("hidden");
    workspace.classList.toggle("history-collapsed", hidden);
    historyToggle.setAttribute("aria-expanded", hidden ? "false" : "true");
  }

  async function initializeHistory() {
    try {
      const summaries = await appCall("ListConversations");
      if (summaries && summaries.length) {
        await openConversation(field(summaries[0], "id", "ID"));
      } else {
        const summary = await appCall("NewConversation");
        activeConversationId = field(summary, "id", "ID");
        await renderHistory();
      }
    } catch (err) {
      appendMessage("assistant", `Could not load local chat history: ${err.message || err}`, { error: true });
    } finally {
      historyReady = true;
      syncSend();
    }
  }

  async function send() {
    const message = input.value.trim();
    if (!message || sendBtn.dataset.busy === "1" || !historyReady) return;

    currentMessages.push({ role: "user", content: message, at: new Date().toISOString() });
    appendMessage("user", message);
    input.value = "";
    resizeInput();
    sendBtn.dataset.busy = "1";
    syncSend();
    const pending = appendMessage("assistant", "Thinking…", { pending: true });

    try {
      const result = await appCall("Chat", message);
      pending.remove();
      const reply = (result && (result.reply || result.Reply)) || "(empty reply)";
      currentMessages.push({ role: "assistant", content: reply, at: new Date().toISOString() });
      appendMessage("assistant", reply);
      await renderHistory();
    } catch (err) {
      pending.remove();
      appendMessage("assistant", err.message || String(err), { error: true });
    } finally {
      sendBtn.dataset.busy = "0";
      syncSend();
      input.focus();
    }
  }

  initializeHistory();

  input.focus();
})();
