// Bot detail page interactions: tabs, settings save, config edit/save.
(function init() {
  const page = document.getElementById("bot-detail-page");
  if (!page) return;

  const botName = page.dataset.botName;
  const botType = page.dataset.botType;

  function renderMessage(message, kind) {
    const err = document.getElementById("settings-error");
    if (!err) return;
    const alert = document.createElement("div");
    alert.className = kind === "success" ? "alert alert-success" : "alert alert-error";
    alert.textContent = String(message);
    err.replaceChildren(alert);
  }

  function renderError(message) {
    renderMessage(message, "error");
  }

  function setupTabs() {
    const tabs = page.querySelectorAll(".detail-tab");
    const panels = page.querySelectorAll(".detail-panel");
    tabs.forEach((tab) => {
      tab.addEventListener("click", () => {
        const key = tab.dataset.tab;
        tabs.forEach((t) => t.classList.toggle("active", t === tab));
        panels.forEach((p) => p.classList.toggle("active", p.dataset.panel === key));
      });
    });
  }

  function setupSettingsSave() {
    const btn = document.getElementById("save-settings-btn");
    if (!btn) return;

    btn.addEventListener("click", async () => {
      btn.disabled = true;
      btn.classList.add("is-busy");

      const persistence = document.getElementById("cfg-persistence")?.checked || false;
      const ingress = document.getElementById("cfg-ingress")?.checked || false;
      const egress = document.getElementById("cfg-egress")?.checked || false;
      const domainsRaw = document.getElementById("cfg-domains")?.value || "";
      const domains = domainsRaw.split("\n").map((s) => s.trim()).filter(Boolean);

      const networkPolicy = {
        ingress: ingress,
        egress: egress,
        useCilium: !egress && domains.length > 0,
        allowedDomains: !egress ? domains : [],
      };

      try {
        const res = await fetch("/bots/" + encodeURIComponent(botName), {
          method: "PUT",
          headers: {
            "Content-Type": "application/json",
            "HX-Request": "true",
          },
          body: JSON.stringify({
            botType: botType,
            values: {
              persistence: { enabled: persistence },
              networkPolicy: networkPolicy,
            },
          }),
        });
        if (!res.ok) {
          renderError(await res.text());
          return;
        }
        window.location.reload();
      } catch (e) {
        renderError("Failed to save settings: " + e);
      } finally {
        btn.disabled = false;
        btn.classList.remove("is-busy");
      }
    });
  }

  function setupConfigEditor() {
    const toggleBtn = document.getElementById("toggle-config-editor-btn");
    const formatBtn = document.getElementById("format-config-btn");
    const validateBtn = document.getElementById("validate-config-btn");
    const cancelBtn = document.getElementById("cancel-config-btn");
    const saveBtn = document.getElementById("save-config-btn");
    const view = document.getElementById("config-view");
    const editor = document.getElementById("config-editor");
    const content = document.getElementById("config-content");
    const tokenButtons = page.querySelectorAll(".copy-secret-token-btn");

    if (!toggleBtn || !view || !editor) return;

    function setEditing(editing) {
      view.style.display = editing ? "none" : "";
      editor.style.display = editing ? "" : "none";
      toggleBtn.style.display = editing ? "none" : "";
    }

    toggleBtn.addEventListener("click", () => setEditing(true));
    if (cancelBtn) {
      cancelBtn.addEventListener("click", () => setEditing(false));
    }

    if (saveBtn && content) {
      saveBtn.addEventListener("click", async () => {
        try {
          JSON.parse(content.value);
        } catch (e) {
          renderError("Config JSON is invalid: " + e);
          return;
        }

        saveBtn.disabled = true;
        saveBtn.classList.add("is-busy");
        try {
          const res = await fetch("/bots/" + encodeURIComponent(botName) + "/config", {
            method: "PUT",
            headers: {
              "Content-Type": "application/json",
              "HX-Request": "true",
            },
            body: JSON.stringify({ configContent: content.value }),
          });
          if (!res.ok) {
            renderError(await res.text());
            return;
          }
          window.location.reload();
        } catch (e) {
          renderError("Failed to save config: " + e);
        } finally {
          saveBtn.disabled = false;
          saveBtn.classList.remove("is-busy");
        }
      });
    }

    if (formatBtn && content) {
      formatBtn.addEventListener("click", () => {
        try {
          const parsed = JSON.parse(content.value);
          content.value = JSON.stringify(parsed, null, 2);
        } catch (e) {
          renderError("Config JSON is invalid: " + e);
        }
      });
    }

    if (validateBtn && content) {
      validateBtn.addEventListener("click", () => {
        try {
          JSON.parse(content.value);
          renderMessage("Config JSON is valid.", "success");
        } catch (e) {
          renderError("Config JSON is invalid: " + e);
        }
      });
    }

    if (tokenButtons.length > 0 && content) {
      tokenButtons.forEach((btn) => {
        btn.addEventListener("click", async () => {
          const token = btn.dataset.secretToken || "";
          if (!token) return;
          try {
            if (navigator.clipboard && typeof navigator.clipboard.writeText === "function") {
              await navigator.clipboard.writeText(token);
              renderMessage("Copied token: " + token, "success");
              return;
            }
          } catch (_) {
            // fallback below
          }
          const start = content.selectionStart ?? content.value.length;
          const end = content.selectionEnd ?? start;
          content.value = content.value.slice(0, start) + token + content.value.slice(end);
          content.focus();
          const cursor = start + token.length;
          content.setSelectionRange(cursor, cursor);
          renderMessage("Inserted token at cursor.", "success");
        });
      });
    }
  }

  function setupBackupConfig(container) {
    const root = container || page;
    const enabled = root.querySelector("#backupEnabled");
    const fields = root.querySelector("#backup-fields");
    const provider = root.querySelector("#backupProvider");
    const s3Fields = root.querySelector("#bk-s3-fields");
    const githubFields = root.querySelector("#bk-github-fields");

    if (enabled && fields) {
      const updateEnabled = () => {
        const visible = enabled.checked;
        fields.style.display = visible ? "" : "none";
        fields.querySelectorAll("input,select,textarea").forEach((el) => {
          el.disabled = !visible;
        });
      };
      enabled.addEventListener("change", updateEnabled);
      updateEnabled();
    }

    if (provider) {
      const updateProvider = () => {
        if (s3Fields) {
          s3Fields.style.display = provider.value === "s3" ? "" : "none";
          s3Fields.querySelectorAll("input,select,textarea").forEach((el) => {
            el.disabled = provider.value !== "s3";
          });
        }
        if (githubFields) {
          githubFields.style.display = provider.value === "github" ? "" : "none";
          githubFields.querySelectorAll("input,select,textarea").forEach((el) => {
            el.disabled = provider.value !== "github";
          });
        }
      };
      provider.addEventListener("change", updateProvider);
      updateProvider();
    }
  }

  setupTabs();
  setupSettingsSave();
  setupConfigEditor();
  setupBackupConfig();

  document.body.addEventListener("htmx:afterSwap", (event) => {
    const target = event.target;
    if (!(target instanceof Element)) return;
    if (target.id === "backup-config-panel" || target.querySelector("#backupEnabled")) {
      setupBackupConfig(target);
    }
  });
})();
