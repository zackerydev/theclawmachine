// Bot install form helpers: conditional fields, provider toggles, model picker.
(function init() {
  function toggle(checkboxId, targetId, showWhen) {
    const cb = document.getElementById(checkboxId);
    const tgt = document.getElementById(targetId);
    if (!cb || !tgt) return;
    const update = () => {
      const visible = showWhen === undefined ? cb.checked : cb.checked === showWhen;
      tgt.style.display = visible ? "" : "none";
      tgt.querySelectorAll("input,select,textarea").forEach((el) => {
        el.disabled = !visible;
      });
    };
    cb.addEventListener("change", update);
    update();
  }

  function providerToggle(selectId, s3Id, githubId) {
    const sel = document.getElementById(selectId);
    if (!sel) return;
    const update = () => {
      const s3 = document.getElementById(s3Id);
      const gh = document.getElementById(githubId);
      if (s3) {
        s3.style.display = sel.value === "s3" ? "" : "none";
        s3.querySelectorAll("input,select,textarea").forEach((el) => {
          el.disabled = sel.value !== "s3";
        });
      }
      if (gh) {
        gh.style.display = sel.value === "github" ? "" : "none";
        gh.querySelectorAll("input,select,textarea").forEach((el) => {
          el.disabled = sel.value !== "github";
        });
      }
    };
    sel.addEventListener("change", update);
    update();
  }

  toggle("persistence", "persistence-size-group");
  toggle("workspaceEnabled", "workspace-fields");
  toggle("backupEnabled", "backup-fields");
  toggle("egress", "allowedDomainsSection", false);

  providerToggle("workspaceProvider", "ws-s3-fields", "ws-github-fields");
  providerToggle("backupProvider", "bk-s3-fields", "bk-github-fields");

  document.querySelectorAll("[data-show-when]").forEach((el) => {
    const [key, value] = el.dataset.showWhen.split("=");
    const src = document.getElementById("cfg-" + key);
    if (!src) return;
    const update = () => {
      const srcVal = src.type === "checkbox" ? String(src.checked) : src.value;
      const visible = srcVal === value;
      el.style.display = visible ? "" : "none";
      el.querySelectorAll("input,select,textarea").forEach((inp) => {
        inp.disabled = !visible;
      });
    };
    src.addEventListener("change", update);
    update();
  });

  const configForm = document.getElementById("config-form");
  const installBtn = document.getElementById("install-bot-btn");
  if (configForm && installBtn) {
    configForm.addEventListener("submit", (e) => {
      const submitter = e.submitter;
      if (submitter && submitter !== installBtn) return;
      installBtn.disabled = true;
      installBtn.classList.add("is-busy");
    });
  }

  const modelCache = new Map();
  function currentBotType() {
    const hidden = document.querySelector('input[name="botType"]');
    return hidden ? hidden.value : "";
  }

  function currentAuthChoice() {
    const auth = document.getElementById("cfg-authChoice");
    return auth ? auth.value : "";
  }

  function modelCatalogKey() {
    return `${currentBotType()}::${currentAuthChoice()}`;
  }

  async function loadModels() {
    const key = modelCatalogKey();
    if (modelCache.has(key)) return modelCache.get(key);
    try {
      const params = new URLSearchParams();
      const botType = currentBotType();
      const authChoice = currentAuthChoice();
      if (botType) params.set("botType", botType);
      if (authChoice) params.set("authChoice", authChoice);
      const query = params.toString();
      const res = await fetch(query ? `/api/models?${query}` : "/api/models");
      const payload = await res.json();
      const models = Array.isArray(payload) ? payload : [];
      modelCache.set(key, models);
      return models;
    } catch {
      modelCache.set(key, []);
      return [];
    }
  }

  document.querySelectorAll("[data-model-picker]").forEach((picker) => {
    const input = picker.querySelector("input");
    const dropdown = picker.querySelector("[data-model-dropdown]");
    if (!input || !dropdown) return;
    let selectedIndex = -1;
    let allowedValues = new Set();

    function renderItems(models) {
      selectedIndex = -1;
      allowedValues = new Set();
      models.forEach((m) => {
        const id = typeof m.id === "string" ? m.id : "";
        if (id) allowedValues.add(id);
      });
      dropdown.replaceChildren();
      if (!models.length) {
        dropdown.classList.remove("show");
        return;
      }
      models.slice(0, 50).forEach((m, i) => {
        const id = typeof m.id === "string" ? m.id : "";
        const name = typeof m.name === "string" ? m.name : id;

        const item = document.createElement("div");
        item.className = "model-item";
        item.dataset.value = id;
        item.dataset.idx = String(i);

        const nameEl = document.createElement("span");
        nameEl.className = "model-name";
        nameEl.textContent = name;
        item.appendChild(nameEl);

        const idEl = document.createElement("span");
        idEl.className = "model-id";
        idEl.textContent = id;
        item.appendChild(idEl);

        dropdown.appendChild(item);
      });
      dropdown.classList.add("show");
    }

    function selectItem(el) {
      input.value = el.dataset.value;
      dropdown.classList.remove("show");
      input.dispatchEvent(new Event("change"));
    }

    dropdown.addEventListener("mousedown", (e) => {
      const item = e.target.closest("[data-value]");
      if (item) selectItem(item);
    });

    async function updateFromInput() {
      const models = await loadModels();
      const q = input.value.toLowerCase();
      if (!q) {
        renderItems(models);
        return;
      }
      renderItems(models.filter((m) => {
        const id = typeof m.id === "string" ? m.id.toLowerCase() : "";
        const name = typeof m.name === "string" ? m.name.toLowerCase() : "";
        return id.includes(q) || name.includes(q);
      }));
    }

    input.addEventListener("focus", updateFromInput);
    input.addEventListener("input", updateFromInput);
    input.addEventListener("keydown", (e) => {
      const items = dropdown.querySelectorAll("[data-value]");
      if (!items.length) return;
      if (e.key === "ArrowDown") {
        e.preventDefault();
        selectedIndex = Math.min(selectedIndex + 1, items.length - 1);
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        selectedIndex = Math.max(selectedIndex - 1, 0);
      } else if (e.key === "Enter" && selectedIndex >= 0) {
        e.preventDefault();
        selectItem(items[selectedIndex]);
      } else if (e.key === "Escape") {
        dropdown.classList.remove("show");
      }
      items.forEach((el, i) => {
        el.classList.toggle("active", i === selectedIndex);
      });
    });

    document.addEventListener("click", (e) => {
      if (!picker.contains(e.target)) dropdown.classList.remove("show");
    });

    const authChoice = document.getElementById("cfg-authChoice");
    if (authChoice) {
      authChoice.addEventListener("change", async () => {
        await updateFromInput();
        if (input.value && !allowedValues.has(input.value)) {
          input.value = "";
          input.setCustomValidity("Select a model from the dropdown for the selected provider.");
          input.reportValidity();
        } else {
          input.setCustomValidity("");
        }
      });
    }

    const form = input.closest("form");
    if (form) {
      form.addEventListener("submit", (e) => {
        if (currentBotType() !== "openclaw") {
          input.setCustomValidity("");
          return;
        }
        if (input.value && !allowedValues.has(input.value)) {
          e.preventDefault();
          input.setCustomValidity("Select a model from the dropdown for the selected provider.");
          input.reportValidity();
          return;
        }
        input.setCustomValidity("");
      });
    }

    updateFromInput().then(() => {
      dropdown.classList.remove("show");
    });
  });
})();
