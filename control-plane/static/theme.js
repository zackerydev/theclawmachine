(function () {
  const THEME_KEY = "clawmachine-theme";
  const DEFAULT_THEME = "dark";
  const THEMES = new Set(["dark", "light"]);

  function getStoredTheme() {
    try {
      const value = localStorage.getItem(THEME_KEY);
      return THEMES.has(value) ? value : null;
    } catch (_) {
      return null;
    }
  }

  function setStoredTheme(theme) {
    try {
      localStorage.setItem(THEME_KEY, theme);
    } catch (_) {
      // Ignore storage errors and keep in-memory theme.
    }
  }

  function currentTheme() {
    const attr = document.documentElement.getAttribute("data-theme");
    if (THEMES.has(attr)) {
      return attr;
    }
    return getStoredTheme() || DEFAULT_THEME;
  }

  function applyTheme(theme) {
    if (!THEMES.has(theme)) {
      return;
    }
    document.documentElement.setAttribute("data-theme", theme);
    setStoredTheme(theme);
    syncButtons(theme);
  }

  function syncButtons(activeTheme) {
    const buttons = document.querySelectorAll("[data-theme-option]");
    buttons.forEach((btn) => {
      const theme = btn.getAttribute("data-theme-option");
      const active = theme === activeTheme;
      btn.setAttribute("aria-pressed", active ? "true" : "false");
      btn.classList.toggle("active", active);
    });
  }

  function initThemeSwitcher() {
    const theme = currentTheme();
    document.documentElement.setAttribute("data-theme", theme);
    syncButtons(theme);

    const buttons = document.querySelectorAll("[data-theme-option]");
    buttons.forEach((btn) => {
      btn.addEventListener("click", () => {
        const selected = btn.getAttribute("data-theme-option");
        applyTheme(selected);
      });
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", initThemeSwitcher);
  } else {
    initThemeSwitcher();
  }
})();
