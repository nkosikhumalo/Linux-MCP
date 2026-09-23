(() => {
  const thread = document.getElementById("thread");
  const input = document.getElementById("input");
  const sendBtn = document.getElementById("send");
  const themeBtn = document.getElementById("theme-btn");
  const themeLabel = document.getElementById("theme-label");

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
    sendBtn.disabled = !input.value.trim() || sendBtn.dataset.busy === "1";
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

  async function callChat(message) {
    const bridge = window.go && window.go.main && window.go.main.App;
    if (!bridge || typeof bridge.Chat !== "function") {
      throw new Error("Backend not connected. Run this UI through Wails (wails dev).");
    }
    return bridge.Chat(message);
  }

  async function send() {
    const message = input.value.trim();
    if (!message || sendBtn.dataset.busy === "1") return;

    appendMessage("user", message);
    input.value = "";
    resizeInput();
    sendBtn.dataset.busy = "1";
    syncSend();

    const pending = appendMessage("assistant", "Thinking…", { pending: true });

    try {
      const result = await callChat(message);
      pending.remove();
      const reply = (result && (result.reply || result.Reply)) || "(empty reply)";
      appendMessage("assistant", reply);
    } catch (err) {
      pending.remove();
      appendMessage("assistant", err.message || String(err), { error: true });
    } finally {
      sendBtn.dataset.busy = "0";
      syncSend();
      input.focus();
    }
  }

  input.focus();
})();
