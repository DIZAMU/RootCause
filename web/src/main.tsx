import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import "./fonts.css";
import "./tokens.css";
import "./styles.css";

// Apply a stored theme before the first paint, so dark mode never flashes
// light. useTheme keeps it in sync afterwards.
try {
  const stored = window.localStorage.getItem("rootcause.theme");
  if (stored === "light" || stored === "dark") document.documentElement.setAttribute("data-theme", stored);
} catch {
  // storage blocked: the system setting applies
}

const root = document.getElementById("root");
if (root) {
  createRoot(root).render(
    <StrictMode>
      <App />
    </StrictMode>,
  );
}
