import { useEffect, useState } from "react";

type Theme = "light" | "dark";

const themeKey = "wmam.ui.theme";

function readInitialTheme(): Theme {
  const stored = localStorage.getItem(themeKey);
  return stored === "dark" ? "dark" : "light";
}

function applyTheme(theme: Theme) {
  document.documentElement.classList.toggle("dark", theme === "dark");
  localStorage.setItem(themeKey, theme);
}

export function applyStoredTheme() {
  applyTheme(readInitialTheme());
}

export function useTheme() {
  const [theme, setTheme] = useState<Theme>(readInitialTheme);

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  function toggleTheme() {
    setTheme((current) => (current === "dark" ? "light" : "dark"));
  }

  return {
    theme,
    isDark: theme === "dark",
    toggleTheme
  };
}
