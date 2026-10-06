(() => {
  const thread = document.getElementById("thread");
  const input = document.getElementById("input");
  const sendBtn = document.getElementById("send");
  const stopBtn = document.getElementById("stop");
  const themeBtn = document.getElementById("theme-btn");
  const themeLabel = document.getElementById("theme-label");
  const newChatBtn = document.getElementById("new-chat");
  const historyToggle = document.getElementById("history-toggle");
  const historyClose = document.getElementById("history-close");
  const historyBackdrop = document.getElementById("history-backdrop");
  const historyPanel = document.getElementById("history-panel");
  const historyList = document.getElementById("history-list");
  const workspace = document.querySelector(".workspace");
  let activeConversationId = null;
  let currentMessages = [];
  let historyReady = false;
  const liveScanReports = new Map();

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
    const busy = sendBtn.dataset.busy === "1";
    sendBtn.hidden = busy;
    stopBtn.hidden = !busy;
    sendBtn.disabled = !input.value.trim() || busy || !historyReady;
    stopBtn.disabled = !busy || stopBtn.dataset.stopping === "1";
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
  stopBtn.addEventListener("click", cancelChat);
  newChatBtn.addEventListener("click", startNewConversation);
  historyToggle.addEventListener("click", toggleHistory);
  historyClose.addEventListener("click", closeHistory);
  historyBackdrop.addEventListener("click", closeHistory);
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape") closeHistory();
  });

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

  function showSecurityScanEvent(event) {
    const scanID = field(event, "id", "ID");
    const kind = field(event, "type", "Type");
    if (!scanID || !kind) return;

    let report = liveScanReports.get(scanID);
    if (!report) {
      const card = el("section", "scan-report");
      const heading = el("div", "scan-report-heading", "ClamAV scan");
      const target = el("div", "scan-report-target", field(event, "path", "Path") || "");
      const status = el("div", "scan-report-status", "Starting scan…");
      const findings = el("div", "scan-findings");
      card.append(heading, target, status, findings);
      thread.appendChild(card);
      report = { card, status, findings, count: 0 };
      liveScanReports.set(scanID, report);
    }

    const message = field(event, "message", "Message") || "";
    if (kind === "finding") {
      report.count++;
      report.status.textContent = `${report.count} finding${report.count === 1 ? "" : "s"} reported so far — scan still running`;
      report.findings.appendChild(el("div", "scan-finding", message));
    } else if (kind === "started") {
      report.status.textContent = "Scanning… findings will appear here as ClamAV reports them.";
    } else {
      const labels = {
        completed: "Scan completed",
        infected: "Scan completed with detections",
        cancelled: "Scan stopped — results are partial",
        incomplete: "Scan incomplete — results are partial",
        failed: "Scan failed",
      };
      report.status.textContent = labels[kind] || message || kind;
      report.status.classList.add(`scan-status-${kind}`);
      if (message) report.status.title = message;
    }
    thread.scrollTop = thread.scrollHeight;
  }

  if (window.runtime && typeof window.runtime.EventsOn === "function") {
    window.runtime.EventsOn("security-scan-update", showSecurityScanEvent);
    window.runtime.EventsOn("tool-approval-request", (event) => {
      const id = field(event, "id", "ID");
      const name = field(event, "name", "Name") || "unknown tool";
      const args = field(event, "arguments", "Arguments") || {};
      const description = field(event, "description", "Description") || "";
      const detail = JSON.stringify(args, null, 2);
      const risk = name === "go_test"
        ? "\n\nWARNING: tests can execute project code with your account permissions."
        : name === "open_uri"
          ? "\n\nThis opens the requested file or URL using your desktop default handler."
          : "";
      const approved = window.confirm(`The assistant requests ${name}.\n${description}${risk}\n\nArguments:\n${detail}\n\nApprove this one action?`);
      appCall("ApproveToolCall", id, approved).catch((err) => {
        appendMessage("assistant", `Could not record approval: ${err.message || err}`, { error: true });
      });
    });
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
      liveScanReports.clear();
      await renderHistory();
      closeHistory();
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
      liveScanReports.clear();
      for (const msg of currentMessages) {
        appendMessage(field(msg, "role", "Role"), field(msg, "content", "Content"));
      }
      await renderHistory();
      closeHistory();
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
      const exportBtn = el("button", "history-action", "Export JSON");
      exportBtn.type = "button";
      exportBtn.title = "Export conversation as JSON";
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
      await appCall("ExportConversation", id);
    } catch (err) {
      appendMessage("assistant", err.message || String(err), { error: true });
    }
  }

  function toggleHistory() {
    if (historyPanel.classList.contains("open")) closeHistory();
    else {
      historyPanel.classList.add("open");
      historyPanel.setAttribute("aria-hidden", "false");
      historyBackdrop.classList.add("visible");
      historyToggle.setAttribute("aria-expanded", "true");
    }
  }

  function closeHistory() {
    historyPanel.classList.remove("open");
    historyPanel.setAttribute("aria-hidden", "true");
    historyBackdrop.classList.remove("visible");
    historyToggle.setAttribute("aria-expanded", "false");
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

  async function cancelChat() {
    stopBtn.dataset.stopping = "1";
    stopBtn.textContent = "Stopping…";
    stopBtn.disabled = true;
    try {
      await appCall("CancelChat");
    } catch (err) {
      appendMessage("assistant", err.message || String(err), { error: true });
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
    const pending = appendMessage("assistant", "Working… this may take a while. Use Stop to cancel.", { pending: true });

    try {
      const result = await appCall("Chat", message);
      pending.remove();
      const reply = (result && (result.reply || result.Reply)) || "(empty reply)";
      currentMessages.push({ role: "assistant", content: reply, at: new Date().toISOString() });
      appendMessage("assistant", reply);
      await renderHistory();
    } catch (err) {
      pending.remove();
      const message = /context canceled/i.test(err.message || "") ? "Stopped." : (err.message || String(err));
      appendMessage("assistant", message, { error: !/^Stopped\.$/.test(message) });
    } finally {
      sendBtn.dataset.busy = "0";
      stopBtn.dataset.stopping = "0";
      stopBtn.textContent = "Stop";
      syncSend();
      input.focus();
    }
  }

  initializeHistory();

  input.focus();
})();
